#!/bin/bash

set -e
set -x

export SHELL=/bin/bash

# Set the flavour environment variable to the string you got in Detecting flavour section.
# For example if the flavour is `amd64-musl` the command will be
export FLAVOUR="amd64-musl"

# Create uploads directory and set proper permissions (skip if planning to use a remote uploader)
# Note: It does not have to be `/var/lib/pleroma/uploads`, the config generator will ask about the upload directory later

mkdir -p /data/var/lib/pleroma/uploads

# Create custom public files directory (custom emojis, frontend bundle overrides, robots.txt, etc.)
# Note: It does not have to be `/var/lib/pleroma/static`, the config generator will ask about the custom public files directory later
mkdir -p /data/var/lib/pleroma/static
chown -R pleroma /data/var/lib/pleroma

# Run the config generator
if ! [ -d /data/tmp ] ; then
	mkdir /data/tmp
fi
chmod 0777 /data/tmp
chmod u+t /data/tmp
su -s $SHELL pleroma -lc "./bin/pleroma_ctl instance gen --output /etc/pleroma/config.exs --output-psql /data/tmp/setup_db.psql"

# Start Postgres
su -s $SHELL postgres -c 'pg_ctl -D /data/var/lib/postgresql/data start'

# Create the postgres database
su -s $SHELL postgres -lc "psql -f /data/tmp/setup_db.psql"

# Create the database schema
su -s $SHELL pleroma -lc "./bin/pleroma_ctl migrate"

# If you have installed RUM indexes uncommend and run
# su pleroma -s $SHELL -lc "./bin/pleroma_ctl migrate --migrations-path priv/repo/optional_migrations/rum_indexing/"

# Start the instance to verify that everything is working as expected
su -s $SHELL pleroma -lc "./bin/pleroma daemon"

# Wait for about 30 seconds and query the instance endpoint, if it shows your uri, name and email correctly, you are configured correctly
( sleep 30 && curl http://localhost:4000/api/v1/instance ) || ( echo why did pleroma not start\? && false )

# Stop the instance
su -s $SHELL pleroma -lc "./bin/pleroma stop"

echo here is your config, copy it to config.exs locally. keep it safe\! there are passwords\!
cat /etc/pleroma/config.exs
echo all good, copy above config to config.exs locally and fly deploy.

