import secrets
from datetime import datetime, timezone


def inject_state():
    with open("state_key.md", "r", encoding="utf-8") as f:
        state_content = f.read()

    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    session_id = secrets.token_hex(4) + "-" + secrets.token_hex(2)

    print('<cartographer_protocol version="0.1">')
    print("<meta>")
    print(f"<session_id>{session_id}</session_id>")
    print(f"<timestamp>{timestamp}</timestamp>")
    print("</meta>")
    print("<state_key>")
    print(state_content)
    print("</state_key>")
    print("</cartographer_protocol>")


if __name__ == "__main__":
    inject_state()
