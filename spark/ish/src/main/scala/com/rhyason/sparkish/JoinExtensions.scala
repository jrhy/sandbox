package com.rhyason.sparkish

import scala.collection.TraversableLike
import scala.collection.generic.CanBuildFrom

object JoinExtensions {
  implicit class FromTraversableLike[Key, Left, Right, Repr](
      val left: TraversableLike[(Key, Left), Repr]
  ) {
    def join[That](
        right: Iterable[(Key, Right)]
    )(implicit bf: CanBuildFrom[Repr, (Key, Left, Right), That]): That =
      left.flatMap { case (k1, v1) =>
        right.flatMap { case (k2, v2) =>
          if (k1 == k2) {
            Some((k1, v1, v2))
          } else {
            None
          }
        }
      }

    def leftJoin[That](
        right: Iterable[(Key, Right)]
    )(implicit bf: CanBuildFrom[Repr, (Key, Left, Option[Right]), That]): That =
      left.flatMap {
        case (k1, v1) => {
          val res = right.flatMap { case (k2, v2) =>
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

    def outerJoin[That](
        right: TraversableLike[(Key, Right), _]
    )(implicit
        bf: CanBuildFrom[Repr, (Key, Option[Left], Option[Right]), That]
    ): That = {
      val b = bf()

      for ((k1, v1) <- left) {
        var rightEmpty = true
        for ((k2, v2) <- right) {
          if (k1 == k2) {
            rightEmpty = false
            b += ((k1, Some(v1), Some(v2)))
          }
        }
        if (rightEmpty) {
          b += ((k1, Some(v1), None))
        }
      }

      for ((k2, v2) <- right) {
        var leftEmpty = true
        for ((k1, v1) <- left) {
          if (k1 == k2) {
            leftEmpty = false
          }
        }
        if (leftEmpty) {
          b += ((k2, None, Some(v2)))
        }
      }

      b.result
    }
  }
}
