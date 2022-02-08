name := "Lark"

version := "1.0"

scalaVersion := "2.13.7"

ThisBuild / scalafixScalaBinaryVersion :=
  CrossVersion.binaryScalaVersion(scalaVersion.value)

libraryDependencies += "org.scalatest" %% "scalatest" % "3.2.10" % "test"

libraryDependencies += "org.apache.spark" %% "spark-sql" % "3.2.0"

libraryDependencies ++= Seq(
  "dev.optics" %% "monocle-core" % "3.1.0",
  "dev.optics" %% "monocle-macro" % "3.1.0"
)

inThisBuild(
  List(
    semanticdbEnabled := true, // enable SemanticDB
    semanticdbVersion := scalafixSemanticdb.revision // only required for Scala 2.x
  )
)

scalacOptions ++= Seq(
  "-encoding",
  "utf8", // Option and arguments on same line
  "-Xfatal-warnings", // New lines for each options
  "-deprecation",
  "-unchecked",
  //"-Ywarn-unused-import" // required by `RemoveUnused` rule
  "-Xlint:unused"
)

// remember to run scalafix with: lark/scalafixAll RemoveUnused
