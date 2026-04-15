#!/bin/sh
echo "==============================="
echo "  Hello from docksmith-demo!"
echo "==============================="
echo
echo "Environment:"
echo "  APP_NAME = ${APP_NAME}"
echo "  GREETING = ${GREETING}"
echo "  PWD      = /app"
echo
echo "Build log:"
cat ./build.log
echo
echo "Message:"
cat ./message.txt
