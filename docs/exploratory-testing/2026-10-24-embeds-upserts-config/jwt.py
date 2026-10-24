"""Print an HS256 JWT. Usage: python3 -I jwt.py SECRET '{"role":"x"}'"""
import base64, hashlib, hmac, sys
def b64(b): return base64.urlsafe_b64encode(b).rstrip(b"=").decode()
h = b64(b'{"alg":"HS256","typ":"JWT"}'); p = b64(sys.argv[2].encode())
s = b64(hmac.new(sys.argv[1].encode(), f"{h}.{p}".encode(), hashlib.sha256).digest())
print(f"{h}.{p}.{s}")
