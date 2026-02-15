#!/usr/bin/env python3
from pathlib import Path
import socket

def main() -> int:
    base_dir = Path(__file__).resolve().parent
    data_dir = base_dir / "sandbox_data"
    data_file = data_dir / "hello.txt"
    outside_file = base_dir.parent / "outside.txt"

    print(f"base_dir={base_dir}")
    print(f"data_dir={data_dir}")

    print("create data directory")
    data_dir.mkdir(exist_ok=True)

    print("write file inside data directory")
    data_file.write_text("hello from sandbox\n", encoding="utf-8")

    print("read file inside data directory")
    content = data_file.read_text(encoding="utf-8").strip()
    print(f"content='{content}'")

    print("attempt write outside data directory (should fail)")
    try:
        outside_file.write_text("nope\n", encoding="utf-8")
        print("unexpected: write outside succeeded")
    except Exception as exc:
        print(f"expected failure: {exc.__class__.__name__}: {exc}")

    print("attempt read outside data directory (should fail)")
    try:
        _ = outside_file.read_text(encoding="utf-8")
        print("unexpected: read outside succeeded")
    except Exception as exc:
        print(f"expected failure: {exc.__class__.__name__}: {exc}")

    print("attempt network access (should fail)")
    try:
        with socket.create_connection(("example.com", 80), timeout=2):
            print("unexpected: network access succeeded")
    except Exception as exc:
        print(f"expected failure: {exc.__class__.__name__}: {exc}")

    print("attempt localhost network access (should fail)")
    try:
        with socket.create_connection(("127.0.0.1", 80), timeout=2):
            print("unexpected: localhost access succeeded")
    except Exception as exc:
        print(f"expected failure: {exc.__class__.__name__}: {exc}")

    print("remove data directory")
    try:
        data_file.unlink()
        data_dir.rmdir()
    except Exception as exc:
        print(f"cleanup failure: {exc.__class__.__name__}: {exc}")
        return 1

    return 0

if __name__ == "__main__":
    raise SystemExit(main())
