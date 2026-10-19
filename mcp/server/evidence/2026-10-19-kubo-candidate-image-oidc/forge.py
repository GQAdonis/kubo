# forge.py KID OUTFILE: writes a structurally valid RS256 JWT with a random signature (never valid).
import base64, json, os, sys, time
def b(x): return base64.urlsafe_b64encode(x).rstrip(b"=").decode()
h = {"alg": "RS256", "typ": "JWT", "kid": sys.argv[1]}
now = int(time.time())
p = {"iss": "https://oauth.know-me.tools", "aud": "ipfs-console", "sub": "461e5d70-d7a6-48ec-a20e-06b5e0236881", "iat": now, "exp": now + 600}
open(sys.argv[2], "w").write(b(json.dumps(h).encode()) + "." + b(json.dumps(p).encode()) + "." + b(os.urandom(256)))
