package example

import org.scalatest.funsuite.AnyFunSuite
import com.rhyason.sparkish.LazyList

class HelloSuite extends AnyFunSuite {
  test("hello") {
    import com.rhyason.sparkish.StaticEncoders.implicits._

    val employees = LazyList(
      Employee("bob", 1) ::
        Employee("joe", 2) ::
        Employee("kat", 4) ::
        Nil
    ).iterator.foreach(println(_))
  }
}

case class Employee(name: String, department_id: Long)
