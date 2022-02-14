
ThisBuild / scalaVersion := "2.12.15"
ThisBuild / version := "0.1.0-SNAPSHOT"
ThisBuild / organization := "com.example"
ThisBuild / organizationName := "example"

externalResolvers += "sparkish packages" at "https://rhyason.com/repository"
libraryDependencies += "com.rhyason" % "sparkish_2.12" % "0.1-SNAPSHOT"
// is there a way to limit which resolver is used for a dependency?
//libraryDependencies += "com.rhyason" % "sparkish_2.12" % "0.1-SNAPSHOT" from "https://rhyason.com/repository/"

lazy val root = (project in file("."))
  .settings(
    name := "hello-sparkish",
    libraryDependencies += "org.scalatest" %% "scalatest-funsuite" % "3.2.11" % "test"
  )
