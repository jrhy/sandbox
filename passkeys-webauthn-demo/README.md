This is a self-contained site that you can use to see how Passkeys
works. It's nice as a Fly app because in order to meet the requirements
for a "secure context" for Passkeys to be available in the browser,
it needs a TLS connection, which is relatively more a PITA to do
locally.

1. `flyctl launch` to get a new random app name.
2. Set the `PUBLIC_HOSTNAME` environment variable (in fly.toml) to the new app name + `.fly.dev`.
3. `fly deploy`
4. Go to the website.
5. Pick a random "email" and Register / Login / whatever. 
6. Have fun!

Based on https://www.herbie.dev/blog/webauthn-basic-web-client-server/ .

