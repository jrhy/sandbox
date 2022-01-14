import org.scalatest.funsuite.AnyFunSuite

//trait Dataset[A, Repr] extends scala.collection.generic.FilterMonadic[A, Repr] {
trait Dataset[A] extends scala.collection.TraversableOnce[A] {
}

trait JoinableDataset[K,V] {
  def join[W](other: Dataset[(K, W)]): JoinableDataset[K, (V, W)]
}

case class Lark[A](l: List[A]) extends Dataset[A] {
//  def map[B](f: A => B): Lark[B] = ???
//  def filter[B](f: A => Boolean): Lark[A] = ???
//  def flatMap[B](f: A => scala.collection.GenTraversableOnce[B]): Lark[B] = ???

// what class provides ^^ from vv?  (a: TraversableOnce does, but not GenTraversableOnce)
        def isTraversableAgain: Boolean = l.isTraversableAgain
        def toIterator: Iterator[A] = l.toIterator
        def toStream: Stream[A] = l.toStream

        def copyToArray[B >: A](xs: Array[B],start: Int,len: Int): Unit = l.copyToArray(xs,start,len)
        def exists(p: A => Boolean): Boolean = l.exists(p)
        def find(p: A => Boolean): Option[A] = l.find(p)
        def forall(p: A => Boolean): Boolean = l.forall(p)
        def foreach[U](f: A => U): Unit = l.foreach(f)
        def hasDefiniteSize: Boolean = l.hasDefiniteSize
        def isEmpty: Boolean = l.isEmpty
        def seq: scala.collection.TraversableOnce[A] = l.seq
        def toTraversable: Traversable[A] = l.toTraversable

}
object Lark {
  def from[A](o: scala.collection.GenTraversableOnce[A]) = Lark[A](o.toList)
  implicit def from[K, V](o: Keyed[K, V]) = o.l
  implicit class Keyed[K,V](val l: Lark[(K,V)]) extends JoinableDataset[K,V] {
    def join[W](other: Dataset[(K, W)]): Keyed[K, (V, W)] =
      Keyed(from(l.flatMap { case (k, v) => other.find { case (k2, w) => k == k2 }.map { case (k2, w) => (k, (v, w))}}))
/*
// it is kinda inconvenient to have to do this. maybe we can meet the trait requirements with
// an implicit that goes back to a Lark? YES!
        def isTraversableAgain: Boolean = l.isTraversableAgain
        def toIterator: Iterator[(K,V)] = l.toIterator
        def toStream: Stream[(K,V)] = l.toStream

        def copyToArray[B >: (K,V)](xs: Array[B],start: Int,len: Int): Unit = l.copyToArray(xs,start,len)
        def exists(p: ((K,V)) => Boolean): Boolean = l.exists(p)
        def find(p: ((K,V)) => Boolean): Option[(K,V)] = l.find(p)
        def forall(p: ((K,V)) => Boolean): Boolean = l.forall(p)
        def foreach[U](f: ((K,V)) => U): Unit = l.foreach(f)
        def hasDefiniteSize: Boolean = l.hasDefiniteSize
        def isEmpty: Boolean = l.isEmpty
        def seq: scala.collection.TraversableOnce[(K,V)] = l.seq
        def toTraversable: Traversable[(K,V)] = l.toTraversable
*/

  }
}
/*object Keyed {
  // to "overcome" the lack of "case-to-case inheritance", instead of the implicit above
  // could an extractor be used somehow? A: not necessary. just implicit from(other).
  def from[K,V](o: Lark[(K,V)]) = Keyed[K,V](o)
}*/

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
