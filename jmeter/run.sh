#!/bin/sh

set -e # Do not continue if jmeter fails

: ${JMETER:=jmeter}
HEAP="-Xms1g -Xmx1g -XX:MaxMetaspaceSize=256m"
date="$(date +%F-%H-%M-%S)"
JVM_ARGS="-Djmeter.reportgenerator.report_title=Telegram-Token-Performance-Test"
$JMETER -n -t telegram-token.jmx -l "results-$date.csv" -e -o "report-$date"
