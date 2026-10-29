"""Small W3C WebDriver client. Mutating requests are never retried."""

import json
import socket
from urllib.error import HTTPError, URLError
from urllib.request import Request, build_opener, ProxyHandler

REQUEST_TIMEOUT_SECONDS = 45
SESSION_TIMEOUT_SECONDS = 240


class Error(Exception):
    pass


class Client:
    def __init__(self, endpoint, session=None):
        self.endpoint = endpoint.rstrip("/")
        self.session = session
        # Local automation must not pass through HTTP_PROXY.
        self.opener = build_opener(ProxyHandler({}))

    def request(self, method, path, body=None, timeout_seconds=REQUEST_TIMEOUT_SECONDS):
        data = None if body is None else json.dumps(body).encode()
        request = Request(
            self.endpoint + path,
            data=data,
            method=method,
            headers={"Content-Type": "application/json"},
        )
        try:
            with self.opener.open(request, timeout=timeout_seconds) as response:
                return json.load(response)["value"]
        except HTTPError as exc:
            try:
                value = json.load(exc)["value"]
                message = f"{value.get('error')}: {value.get('message')}"
            except (ValueError, KeyError, AttributeError):
                message = f"HTTP {exc.code}"
            raise Error(message) from exc
        except (URLError, TimeoutError, socket.timeout, OSError) as exc:
            raise Error(
                f"Transport failed; operation outcome may be unknown. Do not blindly retry: {exc}"
            ) from exc

    def command(self, method, path, body=None):
        if not self.session:
            raise Error("Not connected; run ipad connect")
        return self.request(method, f"/session/{self.session}{path}", body)

    def execute(self, script, args):
        return self.command("POST", "/execute/sync", {"script": script, "args": [args]})
