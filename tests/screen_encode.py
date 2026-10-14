"""Finite synthetic source only. Never calls the portal or PipeWire."""
import importlib.util
from pathlib import Path
import subprocess
import time
import os

spec = importlib.util.spec_from_file_location('screen_encode', Path(__file__).resolve().parents[1] / 'scripts/screen-encode.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

read, write = os.pipe()
try:
    pending = m.ScreenEncoder.portal(read, 42)
    owned = pending.fd
    assert owned != read and pending.process is None and pending.pipeline is None
    pending.close()
    os.fstat(read)  # The borrowed portal descriptor is not consumed.
    try:
        os.fstat(owned)
        raise AssertionError('owned portal descriptor leaked')
    except OSError:
        pass
finally:
    os.close(read)
    os.close(write)

source = m.Gst.ElementFactory.make('videotestsrc')
source.set_property('num-buffers', 30)
source.set_property('is-live', True)
encoder = m.ScreenEncoder(source, width=322, height=180, fps=30, bitrate=1000)
try:
    units = list(m.access_units(encoder.start()))
    output = b''.join(units)
    encoder.thread.join(timeout=2)
    assert not encoder.thread.is_alive() and encoder.error is None, encoder.error
    assert encoder.process.wait(timeout=2) == 0
    assert encoder.frames >= 25, encoder.frames
    assert len(units) == encoder.frames, (len(units), encoder.frames)
    decoded = subprocess.run(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-f', 'h264',
        '-i', 'pipe:0', '-f', 'rawvideo', '-pix_fmt', 'gray', 'pipe:1'], input=output,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=5)
    assert decoded.returncode == 0, decoded.stderr
    assert len(decoded.stdout) == encoder.frames * 322 * 180
finally:
    encoder.close()
    encoder.close()
assert encoder.closed

import io
class TinyReads(io.BytesIO):
    def read(self, size=-1):
        return super().read(min(size, 2))
assert list(m.access_units(TinyReads(output))) == units
try:
    list(m.access_units(io.BytesIO(b'not annexb'), limit=16))
    raise AssertionError('delimiter-less video accepted')
except RuntimeError:
    pass
try:
    list(m.access_units(io.BytesIO(output), limit=16))
    raise AssertionError('oversized access unit accepted')
except RuntimeError:
    pass
try:
    encoder.start()
    raise AssertionError('automatic capture restart allowed')
except RuntimeError:
    pass

source = m.Gst.ElementFactory.make('videotestsrc')
source.set_property('is-live', True)
encoder = m.ScreenEncoder(source, width=1280, height=720)
try:
    encoder.start()  # Deliberately never drain stdout: stop must break backpressure.
    time.sleep(.4)
    started = time.monotonic()
    encoder.close()
    assert time.monotonic() - started < 3
    assert encoder.process.poll() is not None and not encoder.thread.is_alive()
finally:
    encoder.close()
print('PASS: 30-frame moving synthetic encode/decode, RGB row padding, finite backpressure stop, no restart')
