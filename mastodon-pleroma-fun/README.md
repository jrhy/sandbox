
```
fly apps create --name jrhy2
fly volumes create data -a jrhy2 -r sea -s 1 
fly deploy --build-target setup
fly ssh console 
/opt/pleroma/install-pleroma.sh
-- What domain will your instance use? <appname>.fly.dev
-- What ip will the app listen to? 0.0.0.0
(save config.exs locally with:)
fly ssh sftp get /etc/pleroma/config.exs config.exs
-- You may want to change in config.exs to close registrations before deploying:
  registrations_open: true
to:
  registrations_open: false
fly deploy
```

You can customize the hostname by pointing a CNAME to your <app>.fly.dev.
It appears you must use a CNAME while the cert is provisioned; can use A/AAAA later.

```
fly certs add foo.domain.com
sed -i -- 's/<app>.fly.dev/foo.domain.com/g' config.exs 
fly deploy
```

TODO:

* RUM indexes
* check out Fly's Postgres

