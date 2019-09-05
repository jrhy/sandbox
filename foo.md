Overview
--------

This is a quick tour of the Indelible Log API that uses what\'s already
on your machine.

Prerequisites
-------------

  ------------------ ------------------------------------------------------------------------------------
  Indelible apikey   Please [sign up](mailto:showmethelogs@indelible.systems) for the Developer Preview
  `curl`             for issuing HTTP requests to the Indelible API
  `base64`           for doing base64 encoding/decoding
  *Optional:* `jq`   for filtering and formatting results
  ------------------ ------------------------------------------------------------------------------------

Creating a log {#create}
--------------

Let\'s create a new log with the defaults.

``` {#input}
ENDPOINT=https://log.ndlbl.net:8443
CUSTOMERID=customer-id
APIKEY="api-key"
LOGNAME=`echo -n "Hello, Indelible\!"`
```

``` {#input}
curl -s $ENDPOINT/v1/log/create -X POST -H "Content-Type: application/json" -d "
{
  authinfo: {
    customer_id: \"$CUSTOMERID\",
    apikey: \"$APIKEY\"
  },
  logName: \"$LOGNAME\",
  logGroup: \"unencrypted\"
}"
```

Appending to a log {#upsert}
------------------

Now we can append some entries to the log.

``` {#input}
curl -s $ENDPOINT/v1/log/update -X POST -H "Content-Type: application/json" -d "
{
  authinfo: { customer_id: \"$CUSTOMERID\", apikey: \"$APIKEY\" },
  logName: \"$LOGNAME\",
  updateSource: \"walkthrough\"
}
{type: 'Upsert', key: \"`echo -n foo | base64`\", value: \"`echo -n bar | base64`\"}
{type: 'Upsert', key: \"`echo -n foo2 | base64`\", value: \"`echo -n bar2 | base64`\"}
"
```

To facilitate encryption, and keep Indelible agnostic, the keys and
values are treated as binary, so they are base64-encoded over the wire.

Showing the contents of a log {#diff}
-----------------------------

Let\'s see what it looks like:

``` {#input}
curl -s $ENDPOINT/v1/log/diff -s -X POST -H "Content-Type: application/json" -d "
{
  authinfo: {
    customer_id: \"$CUSTOMERID\",
    apikey: \"$APIKEY\"
  },
  logName: \"$LOGNAME\"
}"
```

    {
        "fromVersion": 0,
        "toVersion": 1,
        "page": [
            {
                "change": "Add",
                "entry": {
                    "key": "Zm9v",
                    "value": "YmFy"
                },
                "version": 1
            },
            {
                "change": "Add",
                "entry": {
                    "key": "Zm9vMg==",
                    "value": "YmFyMg=="
                },
                "version": 1
            }
        ],
        "nextPageOptions": null
    }

We can see the entries we added, shown in base64. We\'re also starting
to see how Indelible presents logs in terms of *versions* and *diffs*,
which we\'ll dig into later.

If we want to summarize the contents of a log, we can filter out
everything but the entries, and base64-decode them with `jq`.

``` {#input}
!! | jq '.page[]|.entry|[(.key|@base64d),(.value|@base64d)]' | jq -c
```

    ["foo","bar"]
    ["foo2","bar2"]

Reactive Logs --- Getting notified about new versions {#longpoll}
-----------------------------------------------------

If we\'re asking for a diff when there isn\'t one yet, we can optionally
wait until it shows up. From another terminal window, you could issue
this update:

``` {#input}
curl -s $ENDPOINT/v1/log/update -X POST -H "Content-Type: application/json" -d "
{
  authinfo: { customer_id: \"$CUSTOMERID\", apikey: \"$APIKEY\" },
  logName: \"$LOGNAME\",
  updateSource: \"walkthrough\"
}
{type: 'ExpectVersion', version: '1'}
{type: 'Upsert', key: \"`echo -n foo3 | base64`\", value: \"`echo -n bar3 | base64`\"}
"
```

While in your first terminal window, this diff request will wait up to a
minute for the new version to show up:

``` {#input}
curl -s $ENDPOINT/v1/log/diff -s -X POST -H "Content-Type: application/json" -d "
{
  authinfo: {
    customer_id: \"$CUSTOMERID\",
    apikey: \"$APIKEY\"
  },
  logName: \"$LOGNAME\",
  fromVersion: 1,
  waitSeconds: 60
}"
```

    {
        "fromVersion": 1,
        "toVersion": 2,
        "page": [
            {
                "change": "Add",
                "entry": {
                    "key": "Zm9vMw==",
                    "value": "YmFyMw=="
                },
                "version": 2
            }
        ],
        "nextPageOptions": null
    }

Tracing the history of a key
----------------------------

If we want to know the previous values of a key, we can take advantage
of the `version` response field to put together the chain of versions
that affected the key.

For example, with 3 separate transactions:

``` {#input}
{type: 'ExpectVersion', version: 2}
{type: 'Upsert', key: \"`echo -n foo | base64`\", value: \"`echo -n bar2 | base64`\"}
```

``` {#input}
{type: 'ExpectVersion', version: 3}
{type: 'Upsert', key: \"`echo -n foo | base64`\", value: \"`echo -n bar3 | base64`\"}
```

``` {#input}
{type: 'ExpectVersion', version: 4}
{type: 'Upsert', key: \"`echo -n somethingElse | base64`\", value: \"`echo -n somethingElse | base64`\"}
```

We can see the diff for the latest version points us to version 4 as
having created the value for `foo`:

``` {#input}
curl -s $ENDPOINT/v1/log/diff -s -X POST -H "Content-Type: application/json" -d "
{
  authinfo: {
    customer_id: \"$CUSTOMERID\",
    apikey: \"$APIKEY\"
  },
  logName: \"$LOGNAME\",
  orderResultsBy: \"key\",
  paginationOptions: {
    skipToKey: \"`echo -n foo | base64`\"
  },
  limit: 1
}"
```

    {
        "fromVersion": 0,
        "toVersion": 5,
        "page": [
            {
                "change": "Add",
                "entry": {
                    "key": "Zm9v",
                    "value": "YmFyMw=="
                },
                "version": 4
            }
        ],

    ...

We can ask for the previous version, 3, to see what `foo` was before
that:

``` {#input}
curl -s $ENDPOINT/v1/log/diff -s -X POST -H "Content-Type: application/json" -d "
{
  authinfo: {
    customer_id: \"$CUSTOMERID\",
    apikey: \"$APIKEY\"
  },
  logName: \"$LOGNAME\",
  orderResultsBy: \"key\",
  fromVersion: 0,
  toVersion: 3,
  paginationOptions: {
    skipToKey: \"`echo -n foo | base64`\"
  },
  limit: 1
}"
```

    {
        "fromVersion": 0,
        "toVersion": 3,
        "page": [
            {
                "change": "Add",
                "entry": {
                    "key": "Zm9v",
                    "value": "YmFyMg=="
                },
                "version": 3
            }
        ],

    ...

The result above shows that version 3 set the value to YmFyMg== (bar2).
How about what happened before version 3?

``` {#input}
curl -s $ENDPOINT/v1/log/diff -s -X POST -H "Content-Type: application/json" -d "
{
  authinfo: {
    customer_id: \"$CUSTOMERID\",
    apikey: \"$APIKEY\"
  },
  logName: \"$LOGNAME\",
  orderResultsBy: \"key\",
  fromVersion: 0,
  toVersion: 2,
  paginationOptions: {
    skipToKey: \"`echo -n foo | base64`\"
  },
  limit: 1
}"
```

    {
        "fromVersion": 0,
        "toVersion": 2,
        "page": [
            {
                "change": "Add",
                "entry": {
                    "key": "Zm9v",
                    "value": "YmFy"
                },
                "version": 1
            }
        ],

    ...

Version 1 is as early as we can go.

Troubleshooting
---------------

### Error: Bad API Key

    {"code":1001,"message":"bad API key","details":{}}

Are you sure your shell-fu is alright? Strings properly escaped? If so,
contact us at youbrokeit*@*indelible.systems for verification (but
please don\'t send your apikey in email).
