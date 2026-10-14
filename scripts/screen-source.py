#!/usr/bin/env python3
"""Own one portal-selected screen source. No encoder or Discord transmission."""
import argparse
import json
import secrets
import sys
import time
import select
import signal
from gi.repository import Gio, GLib


class PortalError(RuntimeError):
    pass


class ScreenSource:
    def __init__(self, bus=None, timeout=30):
        self.bus = bus or Gio.bus_get_sync(Gio.BusType.SESSION, None)
        self.timeout = timeout
        self.session = None
        self.fd = None
        self.node = None
        self.closed_subscription = None

    def request(self, method, signature, values, options):
        token = 'omacord_' + secrets.token_hex(8)
        sender = self.bus.get_unique_name()[1:].replace('.', '_')
        path = '/org/freedesktop/portal/desktop/request/' + sender + '/' + token
        result = []
        subscription = self.bus.signal_subscribe(
            'org.freedesktop.portal.Desktop', 'org.freedesktop.portal.Request', 'Response', path, None,
            Gio.DBusSignalFlags.NONE,
            lambda conn, sender, obj, iface, signal, params: result.append(params.unpack()))
        options = dict(options, handle_token=GLib.Variant('s', token))
        deadline = time.monotonic() + self.timeout
        try:
            reply = self.bus.call_sync(
                'org.freedesktop.portal.Desktop', '/org/freedesktop/portal/desktop',
                'org.freedesktop.portal.ScreenCast', method,
                GLib.Variant(signature, tuple(values) + (options,)),
                GLib.VariantType.new('(o)'), Gio.DBusCallFlags.NONE,
                int(self.timeout * 1000), None)
            if reply.unpack()[0] != path:
                raise PortalError('portal request identity mismatch')
            context = GLib.MainContext.default()
            while not result and time.monotonic() < deadline:
                while context.pending():
                    context.iteration(False)
                time.sleep(.005)
            if not result:
                raise PortalError('screen selection timed out')
            code, data = result[0]
            if code != 0:
                raise PortalError('screen selection cancelled' if code == 1 else 'screen selection failed')
            return data
        except BaseException:
            try:
                self.bus.call_sync('org.freedesktop.portal.Desktop', path,
                    'org.freedesktop.portal.Request', 'Close', None, None,
                    Gio.DBusCallFlags.NONE, 1000, None)
            except GLib.Error:
                pass
            raise
        finally:
            self.bus.signal_unsubscribe(subscription)

    def select(self, parent=''):
        self.close()
        try:
            data = self.request('CreateSession', '(a{sv})', (), {
                'session_handle_token': GLib.Variant('s', 'omacord_' + secrets.token_hex(8))})
            session = data.get('session_handle')
            if not isinstance(session, str) or not session.startswith('/org/freedesktop/portal/desktop/session/'):
                raise PortalError('invalid screen session')
            self.session = session
            self.closed_subscription = self.bus.signal_subscribe(
                'org.freedesktop.portal.Desktop', 'org.freedesktop.portal.Session', 'Closed', session, None,
                Gio.DBusSignalFlags.NONE, lambda *args: self.close())
            self.request('SelectSources', '(oa{sv})', (session,), {
                'types': GLib.Variant('u', 3), 'multiple': GLib.Variant('b', False),
                'cursor_mode': GLib.Variant('u', 2), 'persist_mode': GLib.Variant('u', 0)})
            data = self.request('Start', '(osa{sv})', (session, parent), {})
            streams = data.get('streams', [])
            if len(streams) != 1 or not isinstance(streams[0][0], int) or not 0 < streams[0][0] <= 0xffffffff:
                raise PortalError('portal did not select exactly one valid source')
            if self.session != session:
                raise PortalError('screen session was revoked')
            self.node = streams[0][0]
            return {'node_id': self.node, 'properties': streams[0][1]}
        except BaseException:
            self.close()
            raise

    def open_remote(self):
        if not self.session or not self.node:
            raise PortalError('select a source first')
        if self.fd is not None:
            raise PortalError('screen remote already open')
        session = self.session
        reply, fds = self.bus.call_with_unix_fd_list_sync(
            'org.freedesktop.portal.Desktop', '/org/freedesktop/portal/desktop',
            'org.freedesktop.portal.ScreenCast', 'OpenPipeWireRemote',
            GLib.Variant('(oa{sv})', (session, {})), GLib.VariantType.new('(h)'),
            Gio.DBusCallFlags.NONE, int(self.timeout * 1000), None, None)
        handle = reply.unpack()[0]
        if fds is None or handle < 0 or handle >= fds.get_length():
            raise PortalError('invalid PipeWire descriptor')
        fd = fds.get(handle)
        if self.session != session:
            import os
            os.close(fd)
            raise PortalError('screen session was revoked')
        self.fd = fd
        return self.fd

    def close(self):
        import os
        if self.closed_subscription is not None:
            self.bus.signal_unsubscribe(self.closed_subscription)
            self.closed_subscription = None
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
        session, self.session = self.session, None
        self.node = None
        if session:
            try:
                self.bus.call_sync('org.freedesktop.portal.Desktop', session,
                    'org.freedesktop.portal.Session', 'Close', None, None,
                    Gio.DBusCallFlags.NONE, 1000, None)
            except GLib.Error:
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--select', action='store_true', required=True,
                        help='open the desktop source picker explicitly')
    parser.add_argument('--parent-window', default='')
    args = parser.parse_args()
    source = ScreenSource()
    def stopped(signum, frame):
        raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM, stopped)
    try:
        print(json.dumps(source.select(args.parent_window)), flush=True)
        # The session lives only while its owner keeps stdin open. No automatic
        # OpenPipeWireRemote, encoder, recording, stream-create or retry.
        context = GLib.MainContext.default()
        while source.session:
            while context.pending():
                context.iteration(False)
            if select.select([sys.stdin], [], [], .1)[0]:
                line = sys.stdin.readline()
                if not line or line.strip() == 'stop':
                    break
    except KeyboardInterrupt:
        return 0
    except (PortalError, GLib.Error) as error:
        print(json.dumps({'error': str(error)}), flush=True)
        return 1
    finally:
        source.close()
    return 0


if __name__ == '__main__':
    sys.exit(main())
