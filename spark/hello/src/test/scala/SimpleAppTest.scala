import org.scalatest.funsuite.AnyFunSuite
import org.apache.spark.sql.{SparkSession, DataFrame}

class SimpleAppTest extends AnyFunSuite {
  test("helloSparky") {
    val logFile = "../../go.mod"
    val spark = SparkSession.builder
      .appName("Simple Application Test")
      .master("local[*]")
      .getOrCreate
    val logData = spark.read.textFile(logFile).cache()
    val numAs = logData.filter(line => line.contains("a")).count()
    val numBs = logData.filter(line => line.contains("b")).count()
    println(s"Lines with a: $numAs, Lines with b: $numBs")
    spark.stop()
  }
}
