import org.scalatest.funsuite.AnyFunSuite

trait Dataset[A] extends Iterable[A] {}

trait Joiner[K,V] {
  def join[W](other: Dataset[(K, W)]): Joiner[K, (V, W)]
}

case class Lark[A](l: List[A]) extends Dataset[A] {
//  override def map[B](f: A => B): Lark[B] 
  //override def map[B, Lark[B]](f: A => B)(implicit bf: scala.collection.generic.CanBuildFrom[Iterable[A],B,Lark[B]]): Lark[B] = Lark(l.map(f))
  //override def filter(f: A => Boolean): Lark[A] = Lark(l.filter(f))
  //def flatMap[B](f: A => scala.collection.GenTraversableOnce[B]): Lark[B] = Lark(l.flatMap(f))
  def iterator: Iterator[A] = l.iterator
}
object Lark {
  implicit def from[A](o: scala.collection.GenTraversableOnce[A]) = Lark[A](o.toList)
  implicit def from[K, V](o: Keyed[K, V]) = o.l
  implicit class Keyed[K,V](val l: Lark[(K,V)]) extends Joiner[K,V] {
    def join[W](other: Dataset[(K, W)]): Keyed[K, (V, W)] =
      Keyed(l.flatMap {
        case (k, v) => other.iterator
          .find { case (k2, w) => k == k2 }
          .map { case (k2, w) => (k, (v, w))}})
  }
}

class DatasetTest extends AnyFunSuite {
  test("happy") {
    val l = Lark.from(List(1,2,3))
    assert(l.filter(_ != 2).size == 2)
    // RDDs have ++, what is the idiomatic Scala? ... and should the Keyed just work like the PairRDD implicits?
    //val l2 = l ++ Lark.from(List(4,5,6))
    //assert(l2.filter(_ != 2).size == 4)
    val k = Lark.from(List((1,"one"),(2,"two")))
    val k2 = Lark.from(List((1,"foo"),(2,"bar")))
    val kk2: Lark.Keyed[Int, (String, String)] = k.join(k2)
    assert(kk2.toList == List((1,("one","foo")), (2,("two","bar"))))
  }
}

// for comprehensions can be used with anything that impls withFilter, map, and flatMap

class MixinSyntax extends AnyFunSuite {
  case class Experiment(a: String)
  trait A {
    def toString(): String
  }
  test("adding a Trait to an Object Instance") {
    val x: Experiment = new Experiment(a = "foo") with A
    val a = x.asInstanceOf[A]
    if (true) {
      val x: Experiment with A = new Experiment(a = "foo") with A
      val a: Experiment with A = x
    }
  }
}

case class Foo(x: Int) {
  def `+`(y: Int): Unit = {
    println("You asked to +(" + x + "," + y + ")")
  }
  def `+`(y: Foo): Unit = {
    println("You asked to +(" + x + "," + y + ")")
  }
  override def toString = x.toString
}

trait Result[T, E] {
  def map[U](f: T => U): Result[U, E]
  def mapErr[F](f: E => F): Result[T, F]
  def `?` : T
  def `!` : T
}
case class Ok[T, E](ok: T) extends Result[T, E] {
  def map[U](f: T => U): Result[U, E] = Ok(f(ok))
  def mapErr[F](f: E => F): Result[T, F] = Ok(ok)
  def `?` : T = ok
  def `!` : T = ok
}
case class Err[T, E](err: E) extends Result[T, E] {
  def map[U](f: T => U): Result[U, E] = Err(err)
  def mapErr[F](f: E => F): Result[T, F] = Err(f(err))
  def `?` : T = throw new Exception(err.toString)
  def `!` : T = throw new Exception(err.toString)
}

class FooTest extends AnyFunSuite {
  test("infix") {
    val o = Foo(2)
    Foo(6) + o
  }
  test("result") {
    val r: Result[Int, Any] = Ok(5)
    assert((r match {
      case Ok(ok)   => "yay"
      case Err(err) => "wut"
    }) == "yay")
    assert((r ?) == 5)
    assert(r.? == 5)
    assert((r ?) == 5)
    assert((r !) == 5)
    assert(r.! == 5)
  }
}
