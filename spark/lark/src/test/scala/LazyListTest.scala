import org.apache.spark.sql.SparkSession
import org.scalatest.funsuite.AnyFunSuite
import daedalus._
import daedalus.StaticEncoders.implicits._

class LazyListTest extends AnyFunSuite {
  import Data._

  var fastTests = List.empty[() => Unit]

  val doSpark = true
  private lazy val spark = SparkSession
    .builder()
    .master("local[1]")
    .getOrCreate()
  def testCase[A](
      name: String,
      transformer: LazyList[A],
      expected: Iterable[A],
      expectedIterable: Option[Iterable[A]] = None
  ): Unit = {
    if (doSpark) test(name + "Spark") {
      if (expectedIterable.isEmpty) {
        assert(transformer.dataset()(spark).collect().toList == expected)
      } else {
        assert(
          transformer.dataset()(spark).collect().toList == expectedIterable.get
        )
      }
    }

    fastTests = (
        () =>
          test(name + "Iterator") {
            assert(transformer.iterator.toList == expected)
          }
    ) :: fastTests
  }

  testCase(
    "happyIterator",
    transformer = {
      LazyList(List(1, 2, 3))
        .filter(_ % 2 != 0)
    },
    expected = List(1, 3)
  )

  testCase(
    "join1",
    transformer = {
      import StaticEncoders.implicits._
      val left = LazyList(List((1, "a"), (2, "b"), (3, "c")))
      val right = LazyList(List((1, "aaa"), (2, "bbb"), (3, "ccc")))
      left.join(right)
    },
    expected = List(
      (1, "a", "aaa"),
      (2, "b", "bbb"),
      (3, "c", "ccc")
    )
  )

  testCase(
    "joinByKey",
    transformer = {
      import StaticEncoders.implicits._
      val summaries =
        employees
          .keyBy(_.department_id)
          .join(
            departments
              .keyBy(_.department_id)
          )
          .map { case (_, employee, department) =>
            s"${employee.name} works in ${department.name}"
          }
      summaries
    },
    expected = List(
      "bob works in forensics",
      "joe works in investigations"
    )
  )

  testCase(
    "leftJoin",
    transformer = {
      val summaries =
        LazyList
          .LeftJoinable(
            employees
              .keyBy(_.department_id)
          )
          .leftJoin(
            departments
              .keyBy(
                _.department_id
              ) // TODO: should this be a different name, like .tupleWithKey()...
          )
          .map {
            case (_, employee, Some(department)) =>
              s"${employee.name} works in ${department.name}"
            case (_, employee, None) =>
              s"${employee.name} is without a department"
          }
      summaries
    },
    expected = List(
      "bob works in forensics",
      "joe works in investigations",
      "kat is without a department"
    )
  )

  // -o LazyListTest -- -z outerJoinIterator
  testCase(
    "outerJoin",
    transformer = {
      val summaries =
        LazyList
          .OuterJoinable(
            employees
              .keyBy(_.department_id)
          )
          .outerJoin(
            departments
              .keyBy(
                _.department_id
              )
          )
          .map {
            case (_, Some(employee), Some(department)) =>
              s"${employee.name} works in ${department.name}"
            case (_, Some(employee), None) =>
              s"${employee.name} is without a department"
            case (_, None, Some(department)) =>
              s"nobody is in ${department.name}"
            case (_, None, None) =>
              throw new RuntimeException("what is this for")
          }
      summaries
    },
    expected = List(
      "bob works in forensics",
      "joe works in investigations",
      "kat is without a department",
      "nobody is in complaints"
    ),
    expectedIterable = Some(
      List(
        "bob works in forensics",
        "joe works in investigations",
        "nobody is in complaints",
        "kat is without a department"
      )
    )
  )

  fastTests.foreach(_())
}

case class Department(department_id: Long, name: String)
case class Employee(name: String, department_id: Long)

object Data {
  val employees = LazyList(
    Employee("bob", 1) ::
      Employee("joe", 2) ::
      Employee("kat", 4) ::
      Nil
  )
  val departments = LazyList(
    Department(1, "forensics") ::
      Department(2, "investigations") ::
      Department(3, "complaints") ::
      Nil
  )
}
