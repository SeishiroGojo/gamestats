ThisBuild / scalaVersion := "2.13.14"
ThisBuild / version      := "0.1.0"

lazy val root = (project in file("."))
  .settings(
    name := "gamestats-analysis",
    libraryDependencies ++= Seq(
      "io.circe" %% "circe-core"    % "0.14.9",
      "io.circe" %% "circe-generic" % "0.14.9",
      "io.circe" %% "circe-parser"  % "0.14.9",
      "org.xerial" % "sqlite-jdbc" % "3.49.1.0"
    ),
    Compile / mainClass := Some("gamestats.Main"),
    assembly / mainClass := Some("gamestats.Main"),
    assembly / assemblyJarName := "gamestats-analysis.jar",
    assembly / assemblyMergeStrategy := {
      case PathList("META-INF", xs @ _*) => MergeStrategy.discard
      case _                             => MergeStrategy.first
    }
  )

// Nota: el servidor HTTP usa com.sun.net.httpserver (JDK estándar), sin
// framework web adicional, para mantener el foco del proyecto en el
// procesamiento funcional de eventos. circe se usa solo para JSON.
