package daedalus

import scala.reflect.runtime.universe.TypeTag
import org.apache.spark.sql.catalyst.encoders.encoderFor
import org.apache.spark.sql._

abstract class LazyList[A] extends Iterable[A] {

  // The advantage of implicit parameters is that it defers the
  // requirement for a SparkSession until the dataset is requested;
  // LazyList is a common interface between the app and tests that
  // improves on Spark blahblah.
  def dataset()(implicit
      spark: SparkSession
  ): Dataset[A]

  def map[B: Encoder](f: A => B): LazyList[B] =
    Map[A, B](this, f)
  def flatMap[B: Encoder](f: A => IterableOnce[B]): LazyList[B] =
    FlatMap[A, B](this, f)
  override def filter(f: A => Boolean): LazyList[A] =
    Filter[A](this, f)
}

object LazyList {
  def apply[A: Encoder](i: Iterable[A]): LazyList[A] = Source(i)

  implicit class FromIterable[A: Encoder](val i: Iterable[A]) {
    def toLazyList: LazyList[A] = Source(i)
  }

  implicit class Joinable[K: Encoder, V: Encoder](val i: LazyList[(K, V)]) {
    def join[V2: Encoder](o: Joinable[K, V2]): LazyList[(K, V, V2)] =
      Join(i, o.i)
  }

  implicit class KeyBy[A: Encoder](val l: LazyList[A]) {
    def keyBy[K: Encoder](f: A => K): LazyList[(K, A)] = {
      val outputEncoder =
        Encoders.tuple(encoderFor[K], encoderFor[A])
      l.map(e => (f(e), e))(outputEncoder)
    }
  }

  implicit class LeftJoinable[K: Encoder, V: Encoder](val i: LazyList[(K, V)]) {
    def leftJoin[V2: Encoder: TypeTag](
        o: Joinable[K, V2]
    ): LazyList[(K, V, Option[V2])] =
      LeftJoin(i, o.i)
  }

  implicit class OuterJoinable[K: Encoder, V: Encoder: TypeTag](
      val i: LazyList[(K, V)]
  ) {
    def outerJoin[V2: Encoder: TypeTag](
        o: Joinable[K, V2]
    ): LazyList[(K, Option[V], Option[V2])] =
      OuterJoin(i, o.i)
  }
}

case class Source[A: Encoder](source: Iterable[A]) extends LazyList[A] {
  override def iterator: Iterator[A] =
    source.iterator

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    spark.createDataset(source.toSeq)
}

case class Map[A, B: Encoder](
    input: LazyList[A],
    f: A => B
) extends LazyList[B] {
  override def iterator: Iterator[B] =
    input.iterator.map(f)

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[B] =
    input
      .dataset()
      .map(f)
}

case class Filter[A](
    input: LazyList[A],
    f: A => Boolean
) extends LazyList[A] {
  override def iterator: Iterator[A] =
    input.iterator
      .filter(f)

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    input
      .dataset()
      .filter(f)
}

case class FlatMap[A, B: Encoder](
    input: LazyList[A],
    f: A => IterableOnce[B]
) extends LazyList[B] {
  override def iterator: Iterator[B] =
    input.iterator
      .flatMap(f)

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[B] =
    input
      .dataset()
      .flatMap(f)
}

case class Join[Left: Encoder, Right: Encoder, Key: Encoder](
    leftInput: LazyList[(Key, Left)],
    rightInput: LazyList[(Key, Right)]
) extends LazyList[
      (Key, Left, Right)
    ] {
  def iterator: Iterator[(Key, Left, Right)] =
    leftInput.iterator.flatMap { case (k1, v1) =>
      rightInput.iterator.flatMap { case (k2, v2) =>
        if (k1 == k2) {
          Some((k1, v1, v2))
        } else {
          Option.empty
        }
      }
    }

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[(Key, Left, Right)] = {
    val leftDs = leftInput.dataset()
    val rightDs = rightInput.dataset()
    val joined: Dataset[((Key, Left), (Key, Right))] = leftDs
      .joinWith(rightDs, leftDs("_1") === rightDs("_1"))

    val outputEncoder =
      Encoders.tuple(encoderFor[Key], encoderFor[Left], encoderFor[Right])

    joined.map { case ((key, left), (_, right)) => (key, left, right) }(
      outputEncoder
    )
  }
}

case class LeftJoin[
    Left: Encoder,
    Right: Encoder: TypeTag,
    Key: Encoder
](
    leftInput: LazyList[(Key, Left)],
    rightInput: LazyList[(Key, Right)]
) extends LazyList[
      (Key, Left, Option[Right])
    ] {
  def iterator: Iterator[(Key, Left, Option[Right])] =
    leftInput.iterator.flatMap {
      case (k1, v1) => {
        val res = rightInput.iterator.flatMap { case (k2, v2) =>
          if (k1 == k2) {
            Some((k1, v1, Some(v2)))
          } else {
            Option.empty
          }
        }
        if (res.nonEmpty) {
          res
        } else {
          Some((k1, v1, None))
        }
      }
    }

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[(Key, Left, Option[Right])] = {
    val leftDs = leftInput.dataset()
    val rightDs = rightInput.dataset()
    val joined: Dataset[((Key, Left), (Key, Right))] = leftDs
      .joinWith(rightDs, leftDs("_1") === rightDs("_1"), joinType = "left")
    import StaticEncoders.implicits._
    val rightEncoder = encoderFor[Option[Right]]
    val leftEncoder = encoderFor[Left]
    val keyEncoder = encoderFor[Key]
    val outputEncoder = Encoders.tuple(keyEncoder, leftEncoder, rightEncoder)
    joined.map {
      case ((key, left), (_, right)) => (key, left, Some(right))
      case ((key, left), null)       => (key, left, Option.empty)
    }(outputEncoder)
  }
}

case class OuterJoin[
    Left: Encoder: TypeTag,
    Right: Encoder: TypeTag,
    Key: Encoder
](
    leftInput: LazyList[(Key, Left)],
    rightInput: LazyList[(Key, Right)]
) extends LazyList[
      (Key, Option[Left], Option[Right])
    ] {
  def iterator: Iterator[(Key, Option[Left], Option[Right])] =
    leftInput.iterator
      .flatMap {
        case (k1, v1) => {
          val res = rightInput.iterator.flatMap { case (k2, v2) =>
            if (k1 == k2) {
              Some((k1, Some(v1), Some(v2)))
            } else {
              Option.empty
            }
          }.toList
          if (res.nonEmpty) {
            res
          } else {
            Some((k1, Some(v1), None))
          }
        }
      } ++
      rightInput.iterator.flatMap {
        case (k2, v2) => {
          val res = leftInput.iterator.flatMap { case (k1, v1) =>
            if (k1 == k2) {
              Some((k1, Some(v1), Some(v2)))
            } else {
              Option.empty
            }
          }.toList
          if (res.nonEmpty) {
            None
          } else {
            Some((k2, None, Some(v2)))
          }
        }
      }

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[(Key, Option[Left], Option[Right])] = {
    val leftDs = leftInput.dataset()
    val rightDs = rightInput.dataset()
    val joined: Dataset[((Key, Left), (Key, Right))] = leftDs
      .joinWith(rightDs, leftDs("_1") === rightDs("_1"), joinType = "outer")
    import StaticEncoders.implicits._
    val rightEncoder = encoderFor[Option[Right]]
    val leftEncoder = encoderFor[Option[Left]]
    val keyEncoder = encoderFor[Key]
    val outputEncoder = Encoders.tuple(keyEncoder, leftEncoder, rightEncoder)
    joined.map {
      case ((key, left), (_, right)) => (key, Some(left), Some(right))
      case ((key, left), null)       => (key, Some(left), Option.empty)
      case (null, (key, right))      => (key, Option.empty, Some(right))
    }(outputEncoder)
  }
}
