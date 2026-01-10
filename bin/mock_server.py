#!/usr/bin/env python3
"""
Mock HTTP server for testing install.sh

Usage:
    python3 mock_server.py [port] [fixtures_dir]

The server responds based on URL path:
    /404/*          -> 404 Not Found
    /500/*          -> 500 Server Error
    /slow/*         -> Delays 120 seconds (for timeout testing)
    /fail-then-ok/* -> Fails first 2 requests, succeeds on 3rd
    /*              -> Serves files from fixtures_dir
"""

import http.server
import socketserver
import sys
import os
import threading
import time

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8888
FIXTURES_DIR = sys.argv[2] if len(sys.argv) > 2 else os.path.join(os.path.dirname(__file__), "test_fixtures")

# Track request counts for retry testing
request_counts = {}
request_lock = threading.Lock()


class MockHandler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=FIXTURES_DIR, **kwargs)

    def log_message(self, format, *args):
        # Suppress default logging unless DEBUG env is set
        if os.environ.get("DEBUG"):
            super().log_message(format, *args)

    def do_GET(self):
        path = self.path

        # Simulate 404 Not Found
        if "/404" in path:
            self.send_error(404, "Not Found")
            return

        # Simulate 500 Server Error
        if "/500" in path:
            self.send_error(500, "Internal Server Error")
            return

        # Simulate slow response (for timeout testing)
        if "/slow" in path:
            time.sleep(120)
            self.send_response(200)
            self.end_headers()
            return

        # Simulate intermittent failures (for retry testing)
        if "/fail-then-ok" in path:
            with request_lock:
                count = request_counts.get(path, 0) + 1
                request_counts[path] = count

            # Fail first 2 requests, succeed on 3rd
            if count < 3:
                self.send_error(503, "Service Temporarily Unavailable")
                return
            # On 3rd+ request, serve file normally
            # Strip /fail-then-ok from path
            self.path = path.replace("/fail-then-ok", "") or "/"

        # Simulate checksum mismatch (serve wrong content)
        if "/wrong-checksum" in path:
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.end_headers()
            # Send random content that won't match checksum
            self.wfile.write(b"wrong content " * 100000)
            return

        # Simulate corrupted zip
        if "/corrupted.zip" in path:
            self.send_response(200)
            self.send_header("Content-Type", "application/zip")
            self.end_headers()
            self.wfile.write(b"PK\x03\x04" + b"\x00" * 100)  # Invalid zip header
            return

        # Simulate too small file
        if "/small-file" in path:
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.end_headers()
            self.wfile.write(b"too small")  # < 1MB
            return

        # Serve regular files from fixtures directory
        super().do_GET()


class ReusableTCPServer(socketserver.TCPServer):
    allow_reuse_address = True


def main():
    # Create fixtures dir if it doesn't exist
    os.makedirs(FIXTURES_DIR, exist_ok=True)

    # Create mock binary file (2MB of zeros) if not exists
    mock_binary = os.path.join(FIXTURES_DIR, "mock_binary")
    if not os.path.exists(mock_binary):
        with open(mock_binary, "wb") as f:
            f.write(b"\x00" * (2 * 1024 * 1024))

    with ReusableTCPServer(("", PORT), MockHandler) as httpd:
        print(f"Mock server listening on port {PORT}")
        print(f"Fixtures directory: {FIXTURES_DIR}")
        print("Press Ctrl+C to stop")
        try:
            httpd.serve_forever()
        except KeyboardInterrupt:
            print("\nShutting down...")


if __name__ == "__main__":
    main()
