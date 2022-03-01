package com.rhyason.sparkish

import scala.collection.TraversableOnce
import scala.reflect.runtime.universe.TypeTag
import org.apache.spark.sql.catalyst.encoders.encoderFor
import org.apache.spark.sql._

/** Datasetish is a strongly-typed lazy list like a Spark Dataset, that reduces
  * the likelihood of SparkAnalysisExceptions and is faster to test with by
  * eliminating the SparkSession when testing. Datasetish methods are similar to
  * a Dataset except join() is done with keyed pairs instead of loosely-typed
  * Columns.
  *
  * You'll probably also need to import:
  *
  * import com.rhyason.sparkish.StaticEncoders.implicits._
  */
abstract class Datasetish[A] extends Iterable[A] {

  override def iterator(): Iterator[A]

  def dataset()(implicit spark: SparkSession): Dataset[A]

  def map[B: Encoder](f: A => B): Datasetish[B] =
    Map[A, B](this, f)
  def flatMap[B: Encoder](f: A => TraversableOnce[B]): Datasetish[B] =
    FlatMap[A, B](this, f)
  override def filter(f: A => Boolean): Datasetish[A] =
    Filter[A](this, f)
  def union(o: Datasetish[A]): Datasetish[A] =
    Union(this, o)
  def coalesce(partitions: Int): Datasetish[A] =
    sparkOnly(_.coalesce(partitions))
  def repartition(partitions: Int): Datasetish[A] =
    sparkOnly(_.repartition(partitions))
  def persist(): Datasetish[A] =
    sparkOnly(_.persist)
  def unpersist(blocking: Boolean = false): Datasetish[A] =
    sparkOnly(_.unpersist(blocking))

  private def sparkOnly(f: (Dataset[A]) => Dataset[A]): Datasetish[A] =
    new SparkOnly(this, f)
}

object Datasetish {
  def apply[A: Encoder](i: Iterable[A]): Datasetish[A] = Source(i)
  def apply[A: Encoder](d: Dataset[A]): Datasetish[A] = DatasetSource(d)

  implicit class FromIterable[A: Encoder](val i: Iterable[A]) {
    def toDatasetish: Datasetish[A] = Source(i)
  }
  implicit class FromDataset[A: Encoder](val d: Dataset[A]) {
    def toDatasetish: Datasetish[A] = DatasetSource(d)
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

  implicit class Unkey[K: Encoder, A: Encoder](val l: Datasetish[(K, A)]) {
    def unkey(): Datasetish[A] = Map(l, (rec: (K, A)) => rec._2)
  }

  implicit class Unkey2[K: Encoder, A: Encoder, B: Encoder](
      val l: Datasetish[(K, A, B)]
  ) {
    def unkey(): Datasetish[(A, B)] = {
      val outputEncoder =
        Encoders.tuple(encoderFor[A], encoderFor[B])
      Map(l, (rec: (K, A, B)) => (rec._2, rec._3))(outputEncoder)
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

case class DatasetSource[A: Encoder](source: Dataset[A]) extends Datasetish[A] {
  override def iterator: Iterator[A] =
    ???

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    source
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
    JoinExtensions
      .FromTraversableLike(leftInput.toIterable)
      .join(rightInput)
      .iterator

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
    JoinExtensions
      .FromTraversableLike(leftInput.toIterable)
      .leftJoin(rightInput)
      .iterator

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
    JoinExtensions
      .FromTraversableLike(leftInput.toIterable)
      .outerJoin(rightInput)
      .iterator

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

case class Coalesce[A](
    input: Datasetish[A],
    partitions: Int
) extends Datasetish[A] {
  override def iterator: Iterator[A] =
    input.iterator

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    input
      .dataset()
      .coalesce(partitions)
}

abstract class IterableNop[A](
    input: Datasetish[A]
) extends Datasetish[A] {
  final override def iterator: Iterator[A] =
    input.iterator
}

class SparkOnly[A](
    input: Datasetish[A],
    sparkFunc: (Dataset[A]) => Dataset[A]
) extends IterableNop[A](input) {
  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    sparkFunc(
      input
        .dataset()
    )
}

case class Union[A](
    input1: Datasetish[A],
    input2: Datasetish[A]
) extends Datasetish[A] {
  override def iterator: Iterator[A] =
    input1.iterator ++ input2.iterator

  override def dataset()(implicit
      spark: SparkSession
  ): Dataset[A] =
    input1
      .dataset()
      .union(input2.dataset())
}
