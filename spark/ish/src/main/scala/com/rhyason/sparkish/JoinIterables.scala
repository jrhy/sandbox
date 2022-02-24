package com.rhyason.sparkish

object JoinIterables {
  implicit class LeftJoinIterables[Key, Left, Right](
      val left: Iterable[(Key, Left)]
  ) {
    def leftJoin(right: Iterable[(Key, Right)]): Iterable[(Key, Left, Right)] =
      left.flatMap { case (k1, v1) =>
        right.flatMap { case (k2, v2) =>
          if (k1 == k2) {
            Some((k1, v1, v2))
          } else {
            None
          }
        }
      }
  }
}
