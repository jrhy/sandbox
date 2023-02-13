.DELETE_ON_ERROR:

all: corncob_lowercase.txt 5letter_freq.txt

corncob_lowercase.txt:
	curl -LO http://www.mieliestronk.com/corncob_lowercase.txt

5letter_freq.txt: gwordlist/frequency-all.txt.gz
	gzip -cd < gwordlist/frequency-all.txt.gz | grep -i ' [a-z][a-z][a-z][a-z][a-z] ' | tr '[:upper:]' '[:lower:]' | awk '{print $$1,$$2}' > 5letter_freq.txt

gwordlist/:
	mkdir gwordlist

gwordlist/.git: gwordlist/
	cd gwordlist && git init
	cd gwordlist && git remote add origin https://github.com/hackerb9/gwordlist
	cd gwordlist && git fetch origin e3296aaddc13e989eeae9895fcbb52696889ced4
	cd gwordlist && git reset --hard FETCH_HEAD

gwordlist/frequency-all.txt.gz: gwordlist/.git

foo: all
	gzip -cd < gwordlist/frequency-all.txt.gz  | cut -c12-25 
	select * from gwordlist join corncob using(word) where word not like '%s' order by rank desc limit 100;


words.sqlite:
	echo "create table corncob(word);" | sqlite3 words.sqlite
	cat corncob_lowercase.txt | grep '^.....\r$$' | tr -d '\r' | sed 's/\(.*\)/insert into corncob values("\1");/' | sqlite3 words.sqlite
	echo "create table gwordlist(word,rank);" | sqlite3 words.sqlite
	( cat 5letter_freq.txt |  while read rank word ; do echo "insert into gwordlist(word,rank) values(\"$$word\",$$rank);" ; done ; echo commit ) | sqlite3 words.sqlite -cmd begin


clean:
	rm -fr gwordlist

