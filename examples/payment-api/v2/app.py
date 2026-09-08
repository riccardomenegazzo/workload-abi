import os
import signal
import socket
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# Intentional operational changes for the Workload ABI demo:
# - persistent write outside /tmp
# - extra listening socket
# - child shell process
# - larger memory envelope
# - slower SIGTERM handling
CACHE_DIR = "/var/lib/payment"
os.makedirs(CACHE_DIR, exist_ok=True)
MEMORY_PRESSURE = bytearray(96 * 1024 * 1024)

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path not in ("/", "/health"):
            self.send_response(404)
            self.end_headers()
            return
        with open(os.path.join(CACHE_DIR, "cache"), "a", encoding="utf-8") as f:
            f.write("ok\n")
        subprocess.Popen(["/bin/sh", "-c", "sleep 2"])
        body = b"payment-api v2\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        pass

def auxiliary_listener():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(("0.0.0.0", 9090))
    sock.listen(8)
    while True:
        conn, _ = sock.accept()
        conn.close()

threading.Thread(target=auxiliary_listener, daemon=True).start()
server = ThreadingHTTPServer(("0.0.0.0", 8080), Handler)

def stop(signum, frame):
    def delayed_shutdown():
        time.sleep(8)
        server.shutdown()
    threading.Thread(target=delayed_shutdown, daemon=True).start()

signal.signal(signal.SIGTERM, stop)
server.serve_forever()
server.server_close()
