#!/usr/bin/env python3
"""Exercise actual message playback against a private synthetic HTTP fixture."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import subprocess
import tempfile
import threading

root = Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix="omacord-video-") as directory:
    clip = Path(directory) / "clip.mp4"
    subprocess.run(["ffmpeg", "-v", "error", "-f", "lavfi", "-i",
                    "testsrc2=size=320x180:rate=12", "-t", "4", "-c:v", "libx264",
                    "-pix_fmt", "yuv420p", "-an", "-movflags", "+faststart", str(clip)], check=True)

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            data = clip.read_bytes() if self.path == "/clip.mp4" else b"not a video"
            self.send_response(200)
            self.send_header("Content-Type", "video/mp4")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, *_args):
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 9182), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        result = subprocess.run(["/usr/lib/qt6/bin/qmltestrunner", "-input",
                                 str(root / "tests/video"), "-import", str(root)], timeout=30)
    finally:
        server.shutdown()
        server.server_close()
    raise SystemExit(result.returncode)
