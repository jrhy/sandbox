.DELETE_ON_ERROR:
.ONESHELL:
SHELL := /bin/bash

WORDFREQ_URL = https://raw.githubusercontent.com/aparrish/wordfreq-en-25000/master/wordfreq-en-25000-log.json
CORNC0B_URL = https://www.cs.bilkent.edu.tr/~arpa/corncob_lowercase.txt

all: wordfreq-en-25000-log.json corncob_lowercase.txt 5letter_freq.txt

wordfreq-en-25000-log.json:
	curl -L -o $@ $(WORDFREQ_URL)

corncob_lowercase.txt:
	curl -L -o $@ $(CORNC0B_URL)

5letter_freq.txt: wordfreq-en-25000-log.json corncob_lowercase.txt
	python3 -c "import json,re; data=json.load(open('wordfreq-en-25000-log.json','r',encoding='utf-8')); freq_words=[w.lower() for w,_ in data if re.fullmatch(r'[a-z]{5}', w.lower())]; freq_set=set(freq_words); corncob=[w.strip().lower() for w in open('corncob_lowercase.txt','r',encoding='utf-8') if re.fullmatch(r'[a-z]{5}', w.strip().lower())]; extra=[w for w in corncob if w not in freq_set]; words=freq_words+extra; print('\\n'.join(f'{i} {w}' for i,w in enumerate(words,1)))" > $@

words.sqlite:
	echo "create table corncob(word);" | sqlite3 words.sqlite
	cat corncob_lowercase.txt | grep '^.....\r$$' | tr -d '\r' | sed 's/\(.*\)/insert into corncob values("\1");/' | sqlite3 words.sqlite
	echo "create table gwordlist(word,rank);" | sqlite3 words.sqlite
	( cat 5letter_freq.txt |  while read rank word ; do echo "insert into gwordlist(word,rank) values(\"$$word\",$$rank);" ; done ; echo commit ) | sqlite3 words.sqlite -cmd begin

clean:
	rm -f wordfreq-en-25000-log.json
