package com.rhyason.sparkish

import org.scalatest.funsuite.AnyFunSuite

import com.rhyason.sparkish.StaticEncoders.implicits

class StaticImplicitsTest extends AnyFunSuite {

  test("StaticEncoderImplicits should not be used for a SQLContext") {
    assertThrows[RuntimeException] {
      implicits.localSeqToDatasetHolder(List.empty)(
        implicits.newIntEncoder
      )
    }
  }

}
