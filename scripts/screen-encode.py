#!/usr/bin/env python3
"""Portal-FD video encoder building block. Import only; no capture CLI."""
import os
import subprocess
import threading

import gi
gi.require_version('Gst', '1.0')
from gi.repository import Gst
Gst.init(None)


def access_units(stream, limit=4 << 20):
    """Split the owned AUD-enabled encoder pipe without buffering a video.

    x264 emits an access-unit delimiter before each picture. A changed format
    without delimiters fails explicitly instead of guessing frame boundaries.
    """
    buffered = bytearray()
    scan = 0
    have_aud = False
    while True:
        chunk = stream.read(4096)
        eof = not chunk
        buffered.extend(chunk)
        while scan + 4 <= len(buffered):
            prefix = 4 if buffered[scan:scan + 4] == b'\x00\x00\x00\x01' else (
                3 if buffered[scan:scan + 3] == b'\x00\x00\x01' else 0)
            if not prefix or scan + prefix == len(buffered):
                scan += 1 if not prefix else 0
                if prefix:
                    break
                continue
            if buffered[scan + prefix] & 31 == 9:
                if have_aud:
                    unit = bytes(buffered[:scan])
                    del buffered[:scan]
                    scan = 0
                    if len(unit) > limit:
                        raise RuntimeError('encoded screen access unit exceeds budget')
                    yield unit
                else:
                    if scan != 0:
                        raise RuntimeError('screen encoder did not begin with delimiter')
                    have_aud = True
            scan += prefix
        if len(buffered) > limit + 4096:
            raise RuntimeError('encoded screen access unit exceeds budget')
        if eof:
            if buffered:
                if not have_aud or len(buffered) > limit:
                    raise RuntimeError('invalid or oversized encoded screen unit')
                yield bytes(buffered)
            return


class ScreenEncoder:
    """Own one source pipeline and H.264 encoder; audio is never captured.

    The caller must continuously drain output, tie close to room/portal loss,
    and must explicitly start only after source selection and Start sharing.
    No production code currently invokes this helper.
    """
    def __init__(self, source, width=1280, height=720, fps=30, bitrate=6000, fd=None):
        if not (160 <= width <= 1920 and 90 <= height <= 1080 and
                width % 2 == height % 2 == 0 and 1 <= fps <= 30 and 300 <= bitrate <= 16000):
            raise ValueError('invalid video encoder limits')
        self.fd = fd
        self.pipeline = None
        self.process = None
        self.thread = None
        self.error = None
        self.stopped = threading.Event()
        self.started = False
        self.closed = False
        self.frames = 0
        self.width, self.height, self.fps = width, height, fps
        self.bitrate = bitrate
        self.source = source

    @classmethod
    def portal(cls, fd, node, **settings):
        if not isinstance(node, int) or not 0 < node <= 0xffffffff:
            raise ValueError('invalid PipeWire node')
        owned = os.dup(fd)
        try:
            source = Gst.ElementFactory.make('pipewiresrc')
            if source is None:
                raise RuntimeError('GStreamer pipewiresrc is unavailable')
            source.set_property('fd', owned)
            source.set_property('path', str(node))
            return cls(source, fd=owned, **settings)
        except BaseException:
            os.close(owned)
            raise

    def start(self):
        if self.started or self.closed:
            raise RuntimeError('screen encoder cannot be restarted')
        self.started = True
        try:
            self.process = subprocess.Popen([
                '/usr/bin/ffmpeg', '-hide_banner', '-loglevel', 'error', '-nostdin',
                '-f', 'rawvideo', '-pix_fmt', 'rgb24', '-s', f'{self.width}x{self.height}',
                '-r', str(self.fps), '-i', 'pipe:0', '-an', '-c:v', 'libx264',
                '-threads', '2', '-preset', 'veryfast', '-tune', 'zerolatency',
                '-pix_fmt', 'yuv420p', '-bf', '0', '-g', str(self.fps * 2),
                '-b:v', f'{self.bitrate}k', '-maxrate', f'{self.bitrate}k',
                '-bufsize', f'{self.bitrate}k', '-x264-params', 'aud=1:repeat-headers=1',
                '-f', 'h264', 'pipe:1'], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL, bufsize=0)
            # Two independent raw-frame slots: one appsink sample and at most
            # one frame blocked on the encoder pipe. Never drop encoded refs.
            self.pipeline = Gst.Pipeline.new(None)
            sink = Gst.ElementFactory.make('appsink')
            sink.set_property('sync', False)
            sink.set_property('max-buffers', 1)
            sink.set_property('drop', True)
            elements = [self.source] + [Gst.ElementFactory.make(x) for x in
                ('videoconvert', 'videoscale', 'videorate', 'capsfilter')] + [sink]
            if any(x is None for x in elements):
                raise RuntimeError('GStreamer video conversion dependencies unavailable')
            elements[-2].set_property('caps', Gst.Caps.from_string(
                f'video/x-raw,format=RGB,width={self.width},height={self.height},framerate={self.fps}/1'))
            for element in elements:
                self.pipeline.add(element)
            for a, b in zip(elements, elements[1:]):
                if not a.link(b):
                    raise RuntimeError('screen pipeline link failed')
            if self.pipeline.set_state(Gst.State.PLAYING) == Gst.StateChangeReturn.FAILURE:
                raise RuntimeError('screen pipeline failed to start')
            self.thread = threading.Thread(target=self._run, args=(sink,), name='omacord-screen-encode')
            self.thread.start()
            return self.process.stdout
        except BaseException:
            self.close()
            raise

    def _run(self, sink):
        try:
            while not self.stopped.is_set():
                sample = sink.emit('try-pull-sample', 100 * Gst.MSECOND)
                if sample is None:
                    bus = self.pipeline.get_bus()
                    message = bus.pop_filtered(Gst.MessageType.ERROR)
                    if message:
                        raise RuntimeError('screen source pipeline failed')
                    if sink.get_property('eos'):
                        break
                    continue
                buffer = sample.get_buffer()
                ok, mapped = buffer.map(Gst.MapFlags.READ)
                if not ok:
                    raise RuntimeError('screen frame mapping failed')
                try:
                    # Gst RGB rows have 4-byte alignment, unlike FFmpeg raw RGB.
                    row = self.width * 3
                    stride = (row + 3) & ~3
                    if len(mapped.data) != stride * self.height:
                        raise RuntimeError('unexpected screen frame size')
                    frame = mapped.data if stride == row else b''.join(
                        mapped.data[y * stride:y * stride + row] for y in range(self.height))
                    remaining = memoryview(frame)
                    while remaining and not self.stopped.is_set():
                        n = self.process.stdin.write(remaining)
                        if not n:
                            raise RuntimeError('screen encoder input closed')
                        remaining = remaining[n:]
                    self.frames += 1
                finally:
                    buffer.unmap(mapped)
        except (OSError, RuntimeError) as error:
            if not self.stopped.is_set():
                self.error = str(error)
        finally:
            self.process.stdin.close()
            self.pipeline.set_state(Gst.State.NULL)
            if self.error and self.process.poll() is None:
                self.process.terminate()

    def close(self):
        if self.closed:
            return
        self.closed = True
        self.stopped.set()
        if self.pipeline:
            self.pipeline.set_state(Gst.State.NULL)
        if self.process:
            if self.process.poll() is None:
                self.process.terminate()
                try:
                    self.process.wait(timeout=1)
                except subprocess.TimeoutExpired:
                    self.process.kill()
                    self.process.wait(timeout=1)
            if self.thread:
                self.thread.join(timeout=2)
                if self.thread.is_alive():
                    raise RuntimeError('screen encoder worker failed to stop')
            self.process.stdin.close()
            self.process.stdout.close()
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
