
I've found the "corn cob" list to be far less noisy than `/usr/share/dict/words`:
* https://www.cs.bilkent.edu.tr/~arpa/corncob_lowercase.txt

Word frequency comes from `wordfreq-en-25000` (JSON, sorted by frequency). The Makefile uses that ordering and
then appends any 5-letter corncob words that are missing, so `5letter_freq.txt` remains comprehensive while
still roughly ranked.

Sources:
* https://www.cs.bilkent.edu.tr/~arpa/corncob_lowercase.txt
* https://github.com/aparrish/wordfreq-en-25000

Notes:
* `wordfreq-en-25000` is not filtered for offensive words. Adjust filtering if needed.
