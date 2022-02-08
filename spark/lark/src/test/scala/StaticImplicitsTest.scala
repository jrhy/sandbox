import daedalus.StaticEncoders.implicits
import org.scalatest.funsuite.AnyFunSuite

class StaticImplicitsTest extends AnyFunSuite {

  test("StaticEncoderImplicits should not be used for a SQLContext") {
    assertThrows[RuntimeException] {
      implicits.localSeqToDatasetHolder(List.empty)(
        implicits.newIntEncoder
      )
    }
  }

}
