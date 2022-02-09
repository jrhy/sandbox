name := "sparkish"

scalaVersion := "2.12.15"

ThisBuild / organization := "com.rhyason"
ThisBuild / version := "0.1-SNAPSHOT"

publishTo := Some(MavenCache("local-maven", file("./repository")))

ThisBuild / scalafixScalaBinaryVersion :=
  CrossVersion.binaryScalaVersion(scalaVersion.value)

libraryDependencies += "org.scalatest" %% "scalatest" % "3.2.10" % "test"
libraryDependencies += "org.apache.spark" %% "spark-sql" % "3.1.2"

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

// can run scalafix with: sparkish/scalafixAll
