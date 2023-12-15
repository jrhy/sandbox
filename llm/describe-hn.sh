#!/bin/sh

if [ -n "$1" ]; then
	ARG=$1
else
	ARG=37340314
fi
MODEL=gpt-4-1106-preview
curl -s "https://hn.algolia.com/api/v1/items/$ARG" | \
  jq -r 'recurse(.children[]) | .author + ": " + .text' | \
  llm -m $MODEL 'Summarize the themes of the opinions expressed here, including quotes (with author attribution) where appropriate.'


