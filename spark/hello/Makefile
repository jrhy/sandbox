all:

build/spark-3.2.0-bin-hadoop3.2: build/spark-3.2.0-bin-hadoop3.2.tgz 
	shasum -a 512 spark-3.2.0-bin-hadoop3.2.tgz | grep ebe51a449ebd070be7d3570931044070e53c23076abad233b3c51d45a7c99326cf55805ee0d573e6eb7d6a67cfef1963cd77d6dc07dd2fd70fd60da9d1f79e5e || echo checksum mismatch
	cd build ; tar xvzf build/spark-3.2.0-bin-hadoop3.2.tgz 

build/spark-3.2.0-bin-hadoop3.2.tgz: build
	cd build ; curl -LO https://dlcdn.apache.org/spark/spark-3.2.0/spark-3.2.0-bin-hadoop3.2.tgz

build: 
	mkdir build

clean:
	rm -fr build

run: build/spark-3.2.0-bin-hadoop3.2
	sbt compile
	build/spark-3.2.0-bin-hadoop3.2/bin/spark-submit --class "SimpleApp" \
	  --master 'local[4]' \
	    target/scala-2.12/simple-project_2.12-1.0.jar



