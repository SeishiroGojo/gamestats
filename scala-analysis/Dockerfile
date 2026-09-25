FROM sbtscala/scala-sbt:eclipse-temurin-jammy-21.0.2_13_1.10.1_2.13.14 AS build
WORKDIR /app
COPY . .
RUN sbt assembly

FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=build /app/target/scala-2.13/gamestats-analysis.jar /app/gamestats-analysis.jar
EXPOSE 8081
ENTRYPOINT ["java", "-jar", "/app/gamestats-analysis.jar"]