import signal
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CACHE = "/tmp/payment-cache"

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ("/", "/health"):
            self.send_response(404)
            self.end_headers()
            return
        with open(CACHE, "a", encoding="utf-8") as f:
            f.write("ok\n")
        body = b"payment-api v1\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        pass

server = ThreadingHTTPServer(("0.0.0.0", 8080), Handler)

def stop(signum, frame):
    threading.Thread(target=server.shutdown, daemon=True).start()

signal.signal(signal.SIGTERM, stop)
server.serve_forever()
server.server_close()
