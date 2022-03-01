package com.rhyason.sparkish

import org.scalatest.funsuite.AnyFunSuite
import JoinExtensions._

class JoinExtensionsTest extends AnyFunSuite {
  test("happy") {
    val left = List((1, "one"), (2, "two"), (3, "three"))
    val right = List((1, "a"), (2, "b"), (3, "c"))
    val joined: List[(Int, String, String)] = left.join(right)
    assert(joined == List((1, "one", "a"), (2, "two", "b"), (3, "three", "c")))
  }
}
