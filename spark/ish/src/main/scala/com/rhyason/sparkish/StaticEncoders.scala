package com.rhyason.sparkish

import org.apache.spark.sql.{SQLContext, SQLImplicits}

object StaticEncoders {
  val implicits: SQLImplicits = new SQLImplicits {
    override protected def _sqlContext: SQLContext = throw new RuntimeException(
      "StaticEncoders implicits is not intended to be used in a place where sqlContet is used. Maybe use SparkSession.implicits._ instead."
    )
  }
}
