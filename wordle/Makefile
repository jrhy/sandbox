all: corncob_lowercase.txt 5letter_freq.txt

corncob_lowercase.txt:
	curl -LO http://www.mieliestronk.com/corncob_lowercase.txt

5letter_freq.txt: gwordlist/frequency-all.txt.gz
	gzcat gwordlist/frequency-all.txt.gz | grep -i ' [a-z][a-z][a-z][a-z][a-z] ' | tr '[:upper:]' '[:lower:]' | awk '{print $$1,$$2}' > 5letter_freq.txt

gwordlist/frequency-all.txt.gz:
	git clone https://github.com/hackerb9/gwordlist
	# don't care enough to switch to git-lfs
	cd gwordlist ; git checkout e3296aaddc13e989eeae9895fcbb52696889ced4

foo: all
	gzcat gwordlist/frequency-all.txt.gz  | cut -c12-25 
	select * from gwordlist join corncob using(word) where word not like '%s' order by rank desc limit 100;


words.sqlite:
	echo "create table corncob(word);" | sqlite3 words
	cat corncob_lowercase.txt | grep '^.....\r$$' | tr -d '\r' | sed 's/\(.*\)/insert into corncob values("\1");/' | sqlite3 words
	echo "create table gwordlist(rank,word);" | sqlite3 words
	cat 5letter_freq.txt |  while read rank word ; do echo "insert into gwordlist(rank,word) values($$rank,\"$$word\");" ; done | tee /dev/tty | sqlite3 words



