#!/usr/bin/env python3
"""Real private D-Bus portal contract replay, never the desktop bus."""
import importlib.util
import os
from pathlib import Path
import threading
import time
from gi.repository import Gio, GLib

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('screen_source', ROOT / 'scripts/screen-source.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

XML = '''<node><interface name="org.freedesktop.portal.ScreenCast">
<method name="CreateSession"><arg type="a{sv}" direction="in"/><arg type="o" direction="out"/></method>
<method name="SelectSources"><arg type="o" direction="in"/><arg type="a{sv}" direction="in"/><arg type="o" direction="out"/></method>
<method name="Start"><arg type="o" direction="in"/><arg type="s" direction="in"/><arg type="a{sv}" direction="in"/><arg type="o" direction="out"/></method>
<method name="OpenPipeWireRemote"><arg type="o" direction="in"/><arg type="a{sv}" direction="in"/><arg type="h" direction="out"/></method>
</interface><interface name="org.freedesktop.portal.Session"><method name="Close"/></interface>
<interface name="org.freedesktop.portal.Request"><method name="Close"/></interface></node>'''

context = GLib.MainContext.new()
started = threading.Event()
state = {'mode': 'ok', 'calls': [], 'session': None, 'held': None}

def server():
    context.push_thread_default()
    bus = Gio.DBusConnection.new_for_address_sync(os.environ['DBUS_SESSION_BUS_ADDRESS'],
        Gio.DBusConnectionFlags.AUTHENTICATION_CLIENT | Gio.DBusConnectionFlags.MESSAGE_BUS_CONNECTION, None, None)
    bus.call_sync('org.freedesktop.DBus', '/org/freedesktop/DBus', 'org.freedesktop.DBus', 'RequestName',
                  GLib.Variant('(su)', ('org.freedesktop.portal.Desktop', 0)), None, Gio.DBusCallFlags.NONE, 1000, None)
    interfaces = Gio.DBusNodeInfo.new_for_xml(XML).interfaces
    def method(conn, sender, path, iface, name, params, invocation):
        state['calls'].append(name)
        if name == 'Close':
            invocation.return_value(None)
            return
        if name == 'OpenPipeWireRemote':
            fd = os.open('/dev/null', os.O_RDONLY)
            try:
                fds = Gio.UnixFDList.new()
                index = fds.append(fd)
                invocation.return_value_with_unix_fd_list(GLib.Variant('(h)', (index,)), fds)
            finally:
                os.close(fd)
            return
        values = params.unpack(); options = values[-1]
        target = sender[1:].replace('.', '_')
        request = '/org/freedesktop/portal/desktop/request/' + target + '/' + options['handle_token']
        bus.register_object(request, interfaces[2], method, None, None)
        invocation.return_value(GLib.Variant('(o)', (request,)))
        code, data = 0, {}
        if name == 'CreateSession':
            session = '/org/freedesktop/portal/desktop/session/' + target + '/' + options['session_handle_token']
            state['session'] = session
            bus.register_object(session, interfaces[1], method, None, None)
            data = {'session_handle': GLib.Variant('o', session)}
        elif name == 'SelectSources':
            assert options['types'] == 3 and options['multiple'] is False and options['persist_mode'] == 0
        elif name == 'Start':
            if state['mode'] == 'cancel': code = 1
            elif state['mode'] == 'timeout':
                state['held'] = request
                return
            else: data = {'streams': GLib.Variant('a(ua{sv})', [(42, {'source_type': GLib.Variant('u', 2)})])}
        # Emit immediately after return; the client must subscribe before call.
        bus.emit_signal(sender, request, 'org.freedesktop.portal.Request', 'Response', GLib.Variant('(ua{sv})', (code, data)))
    bus.register_object('/org/freedesktop/portal/desktop', interfaces[0], method, None, None)
    state['bus'] = bus
    started.set()
    GLib.MainLoop.new(context, False).run()

threading.Thread(target=server, daemon=True).start()
assert started.wait(3)
source = module.ScreenSource(timeout=.15)
assert source.select()['node_id'] == 42
assert 'OpenPipeWireRemote' not in state['calls'], 'selection started capture automatically'
fd = source.open_remote(); os.fstat(fd)
source.close()
try: os.fstat(fd); raise AssertionError('FD leaked')
except OSError: pass
assert source.session is None
for mode in ['cancel', 'timeout']:
    state['mode'] = mode
    before = state['calls'].count('OpenPipeWireRemote')
    try: source.select(); raise AssertionError('failed request accepted')
    except module.PortalError: pass
    assert source.session is None and state['calls'].count('OpenPipeWireRemote') == before
state['mode'] = 'ok'
source.select(); fd = source.open_remote()
state['bus'].emit_signal(None, source.session, 'org.freedesktop.portal.Session', 'Closed', GLib.Variant('(a{sv})', ({},)))
deadline = time.monotonic()+1
while source.session and time.monotonic()<deadline:
    GLib.MainContext.default().iteration(False); time.sleep(.005)
assert source.session is None
try: os.fstat(fd); raise AssertionError('revoke FD leaked')
except OSError: pass
print('PASS: private portal selection/early response/cancel/timeout/revoke/FD cleanup; no desktop capture or Discord')
