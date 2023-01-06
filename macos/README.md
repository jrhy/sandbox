Swift scripts for accessing the Keychain. 

These are OK for reading and maybe sharing via iCloud with MacOS
but not iOS, as iOS doesn't seem to show "Internet password"s
`keychain-create` creates.  Similarly, `keychain-get` would need
more work to see "Web form password"s that Safari makes.

