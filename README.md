# Watch Locally
This project is under construction...

## How to generate SSL keys
```bash
openssl req -newkey rsa:4096  -x509  -sha512  -days 365 -nodes -out certificate.pem -keyout privatekey.pem
```

Git comes with many preinstalled binaries among which is `openssl` <br>
On Windows it can be found at `Git/usr/bin/openssl.exe` where `Git` is git's root installation directory

