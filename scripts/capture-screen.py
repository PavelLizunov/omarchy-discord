#!/usr/bin/env python3
"""Capture one portal-selected source as H.264 on stdout; never join Discord.

Requires PyGObject and GStreamer with PipeWire, x264 and H.264 parsing elements.
The portal owns selection and permission; GStreamer owns conversion, encoding,
buffering and pipe output. The backend owns the optional Discord sender.
"""
import argparse
import os
import signal
import struct
import sys
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--height', type=int, choices=(360, 720, 1080), default=720)
    parser.add_argument('--fps', type=int, choices=(15, 30), default=30)
    parser.add_argument('--framed', action='store_true', help='emit length-prefixed H.264 access units')
    args = parser.parse_args()

    import gi
    gi.require_version('Gst', '1.0')
    gi.require_version('GstVideo', '1.0')
    from gi.repository import Gio, GLib, Gst, GstVideo

    Gst.init(None)
    for name in ('pipewiresrc', 'queue', 'videoconvert', 'videoscale', 'videorate',
                 'x264enc', 'h264parse', 'appsink' if args.framed else 'fdsink'):
        if Gst.ElementFactory.find(name) is None:
            raise RuntimeError('Missing GStreamer element: ' + name)

    bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    destination = 'org.freedesktop.portal.Desktop'
    path = '/org/freedesktop/portal/desktop'
    interface = 'org.freedesktop.portal.ScreenCast'
    loop = GLib.MainLoop()
    responses = {}
    session = None
    request = None
    pipeline = None
    remote_fd = None
    stopped = False
    failure = None

    def stop(message=None):
        nonlocal stopped, failure
        stopped = True
        if message and failure is None:
            failure = message
        loop.quit()
        return GLib.SOURCE_REMOVE

    def response(_bus, _sender, object_path, _interface, _signal, parameters):
        if object_path == request:
            responses[object_path] = parameters.unpack()
            loop.quit()

    def session_closed(_bus, _sender, object_path, *_):
        if object_path == session:
            stop('Screen sharing permission ended')

    subscription = bus.signal_subscribe(destination, 'org.freedesktop.portal.Request',
                                        'Response', None, None, Gio.DBusSignalFlags.NONE,
                                        response)
    closed_subscription = bus.signal_subscribe(destination, 'org.freedesktop.portal.Session',
                                               'Closed', None, None, Gio.DBusSignalFlags.NONE,
                                               session_closed)

    def portal(method, parameters):
        nonlocal request
        if stopped:
            raise RuntimeError(failure or 'Capture cancelled')
        token = parameters.unpack()[-1]['handle_token']
        sender = bus.get_unique_name()[1:].replace('.', '_')
        request = '/org/freedesktop/portal/desktop/request/' + sender + '/' + token
        result = bus.call_sync(destination, path, interface, method, parameters,
                               GLib.VariantType.new('(o)'), Gio.DBusCallFlags.NONE, 10000, None)
        request = result.unpack()[0]
        timeout = GLib.timeout_add_seconds(120, lambda: stop('Screen selection timed out'))
        try:
            while request not in responses and not stopped:
                loop.run()
            if stopped:
                raise RuntimeError(failure or 'Capture cancelled')
            code, values = responses.pop(request)
            request = None
            if code != 0:
                raise RuntimeError('Screen selection cancelled or rejected')
            return values
        finally:
            if GLib.MainContext.default().find_source_by_id(timeout):
                GLib.source_remove(timeout)

    signals = [GLib.unix_signal_add(GLib.PRIORITY_DEFAULT, number, stop)
               for number in (signal.SIGINT, signal.SIGTERM)]
    try:
        token = 'omacord_' + uuid.uuid4().hex
        session = portal('CreateSession', GLib.Variant('(a{sv})', ({
            'handle_token': GLib.Variant('s', token),
            'session_handle_token': GLib.Variant('s', token + '_session'),
        },)))['session_handle']
        portal('SelectSources', GLib.Variant('(oa{sv})', (session, {
            'handle_token': GLib.Variant('s', token + '_select'),
            'types': GLib.Variant('u', 3),
            'multiple': GLib.Variant('b', False),
            'cursor_mode': GLib.Variant('u', 2),
            'persist_mode': GLib.Variant('u', 0),
        })))
        selected = portal('Start', GLib.Variant('(osa{sv})', (session, '', {
            'handle_token': GLib.Variant('s', token + '_start'),
        })))
        streams = selected.get('streams', [])
        if len(streams) != 1:
            raise RuntimeError('Select exactly one screen or window')
        node, properties = streams[0]
        reply, descriptors = bus.call_with_unix_fd_list_sync(
            destination, path, interface, 'OpenPipeWireRemote',
            GLib.Variant('(oa{sv})', (session, {})), GLib.VariantType.new('(h)'),
            Gio.DBusCallFlags.NONE, 10000, None, None)
        remote_fd = descriptors.get(reply.unpack()[0])
        width, height = args.height * 16 // 9, args.height
        pipeline = Gst.parse_launch(
            'pipewiresrc name=capture do-timestamp=true ! '
            'queue max-size-buffers=2 max-size-bytes=0 max-size-time=0 leaky=downstream ! '
            'videoconvert ! videoscale add-borders=true ! videorate drop-only=true ! '
            f'video/x-raw,format=I420,width={width},height={height},'
            f'pixel-aspect-ratio=1/1,framerate={args.fps}/1 ! '
            f'x264enc name=encoder tune=zerolatency speed-preset=veryfast bitrate=2500 '
            f'key-int-max={args.fps * 2} byte-stream=true ! '
            'video/x-h264,profile=constrained-baseline,stream-format=byte-stream,alignment=au ! '
            'h264parse config-interval=-1 ! ' +
            ('appsink name=output emit-signals=true max-buffers=1 drop=false sync=false'
             if args.framed else 'fdsink fd=1 sync=false'))
        if args.framed:
            def encoded_sample(sink):
                sample = sink.emit('pull-sample')
                buffer = sample.get_buffer()
                if buffer.get_size() > 4 * 1024 * 1024:
                    GLib.idle_add(stop, 'Encoded frame exceeds limit')
                    return Gst.FlowReturn.ERROR
                payload = buffer.extract_dup(0, buffer.get_size())
                stamp = buffer.pts if buffer.pts != Gst.CLOCK_TIME_NONE else 0
                try:
                    sys.stdout.buffer.write(struct.pack('!IQ', len(payload), stamp))
                    sys.stdout.buffer.write(payload)
                    sys.stdout.buffer.flush()
                except (BrokenPipeError, OSError):
                    GLib.idle_add(stop)
                    return Gst.FlowReturn.ERROR
                return Gst.FlowReturn.OK
            pipeline.get_by_name('output').connect('new-sample', encoded_sample)

        def keyframe():
            if pipeline is not None:
                event = GstVideo.video_event_new_upstream_force_key_unit(Gst.CLOCK_TIME_NONE, True, 0)
                pipeline.get_by_name('encoder').get_static_pad('src').send_event(event)
            return GLib.SOURCE_CONTINUE

        signals.append(GLib.unix_signal_add(GLib.PRIORITY_DEFAULT, signal.SIGUSR1, keyframe))
        source = pipeline.get_by_name('capture')
        source.set_property('fd', remote_fd)
        if properties.get('pipewire-serial') and source.find_property('target-object'):
            source.set_property('target-object', str(properties['pipewire-serial']))
        else:
            source.set_property('path', str(node))
        messages = pipeline.get_bus()
        messages.add_signal_watch()

        def message(_bus, event):
            if event.type == Gst.MessageType.ERROR:
                stop('Screen capture failed: ' + event.parse_error()[0].message)
            elif event.type == Gst.MessageType.EOS:
                stop()

        messages.connect('message', message)
        if pipeline.set_state(Gst.State.PLAYING) == Gst.StateChangeReturn.FAILURE:
            raise RuntimeError('Cannot start PipeWire capture')
        loop.run()
        if failure:
            raise RuntimeError(failure)
    finally:
        stopped = True
        if pipeline is not None:
            pipeline.set_state(Gst.State.NULL)
        if remote_fd is not None:
            os.close(remote_fd)
        for object_path, object_interface in ((request, 'org.freedesktop.portal.Request'),
                                               (session, 'org.freedesktop.portal.Session')):
            if object_path:
                try:
                    bus.call_sync(destination, object_path, object_interface, 'Close', None,
                                  None, Gio.DBusCallFlags.NONE, 3000, None)
                except GLib.Error:
                    pass
        bus.signal_unsubscribe(subscription)
        bus.signal_unsubscribe(closed_subscription)
        for source_id in signals:
            context = GLib.MainContext.default()
            if context.find_source_by_id(source_id):
                GLib.source_remove(source_id)
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ImportError, OSError, RuntimeError) as error:
        print('capture-screen: ' + str(error), file=sys.stderr)
        sys.exit(1)
