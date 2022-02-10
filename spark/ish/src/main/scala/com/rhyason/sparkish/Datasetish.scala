package com.rhyason.sparkish

import scala.collection.TraversableOnce
import scala.reflect.runtime.universe.TypeTag
import org.apache.spark.sql.catalyst.encoders.encoderFor
import org.apache.spark.sql._

/** Datasetish is a strongly-typed subset of Spark Dataset functionality that
  * reduces the likelihood of SparkAnalysisExceptions and is faster to test with
  * by eliminating the requirement for a SparkSession when testing. Datasetish
  * emulates Spark's laziness and provides similar map(), flatMap(), join(),
  * though without requiring loosely-typed Columns.
  *
  * You'll probably also need to import:
  *
  * import com.rhyason.sparkish.StaticEncoders.implicits._
  */
abstract class Datasetish[A] extends Iterable[A] {

  override def iterator(): Iterator[A]

  def dataset()(implicit
      spark: SparkSession
  ): Dataset[A]

  def map[B: Encoder](f: A => B): Datasetish[B] =
    Map[A, B](this, f)
  def flatMap[B: Encoder](f: A => TraversableOnce[B]): Datasetish[B] =
    FlatMap[A, B](this, f)
  override def filter(f: A => Boolean): Datasetish[A] =
    Filter[A](this, f)
}

object Datasetish {
  def apply[A: Encoder](i: Iterable[A]): Datasetish[A] = Source(i)

  implicit class FromIterable[A: Encoder](val i: Iterable[A]) {
    def toLazyList: Datasetish[A] = Source(i)
  }

  implicit class Joinable[K: Encoder, V: Encoder](val i: Datasetish[(K, V)]) {
    def join[V2: Encoder](o: Joinable[K, V2]): Datasetish[(K, V, V2)] =
      Join(i, o.i)
  }

  implicit class KeyBy[A: Encoder](val l: Datasetish[A]) {
    def keyBy[K: Encoder](f: A => K): Datasetish[(K, A)] = {
      val outputEncoder =
        Encoders.tuple(encoderFor[K], encoderFor[A])
      l.map((e: A) => (f(e), e))(outputEncoder)
    }
  }

  implicit class LeftJoinable[K: Encoder, V: Encoder](
      val i: Datasetish[(K, V)]
  ) {
    def leftJoin[V2: Encoder: TypeTag](
        o: Joinable[K, V2]
    ): Datasetish[(K, V, Option[V2])] =
      LeftJoin(i, o.i)
  }

  implicit class OuterJoinable[K: Encoder, V: Encoder: TypeTag](
      val i: Datasetish[(K, V)]
  ) {
    def outerJoin[V2: Encoder: TypeTag](
        o: Joinable[K, V2]
    ): Datasetish[(K, Option[V], Option[V2])] =
      OuterJoin(i, o.i)
  }
}

case class Source[A: Encoder](source: Iterable[A]) extends Datasetish[A] {
  override def iterator: Iterator[A] =
    source.iterator

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    spark.createDataset(source.toSeq)
}

case class Map[A, B: Encoder](
    input: Datasetish[A],
    f: A => B
) extends Datasetish[B] {
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
    input: Datasetish[A],
    f: A => Boolean
) extends Datasetish[A] {
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
    input: Datasetish[A],
    f: A => TraversableOnce[B]
) extends Datasetish[B] {
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
    leftInput: Datasetish[(Key, Left)],
    rightInput: Datasetish[(Key, Right)]
) extends Datasetish[
      (Key, Left, Right)
    ] {
  def iterator: Iterator[(Key, Left, Right)] =
    leftInput.iterator.flatMap { case (k1, v1) =>
      rightInput.iterator.flatMap { case (k2, v2) =>
        if (k1 == k2) {
          Some((k1, v1, v2))
        } else {
          None
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
    leftInput: Datasetish[(Key, Left)],
    rightInput: Datasetish[(Key, Right)]
) extends Datasetish[
      (Key, Left, Option[Right])
    ] {
  def iterator: Iterator[(Key, Left, Option[Right])] =
    leftInput.iterator.flatMap {
      case (k1, v1) => {
        val res = rightInput.iterator.flatMap { case (k2, v2) =>
          if (k1 == k2) {
            Some((k1, v1, Some(v2)))
          } else {
            None
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
      case ((key, left), null)       => (key, left, None)
    }(outputEncoder)
  }
}

case class OuterJoin[
    Left: Encoder: TypeTag,
    Right: Encoder: TypeTag,
    Key: Encoder
](
    leftInput: Datasetish[(Key, Left)],
    rightInput: Datasetish[(Key, Right)]
) extends Datasetish[
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
              None
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
              None
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
      case ((key, left), null)       => (key, Some(left), None)
      case (null, (key, right))      => (key, Option.empty, Some(right))
    }(outputEncoder)
  }
}
