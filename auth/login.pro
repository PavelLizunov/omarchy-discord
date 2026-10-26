QT += widgets webenginewidgets network
!versionAtLeast(QT_VERSION, 6.9.0): error("Discord sign-in requires Qt 6.9 or newer")
CONFIG += c++17
TARGET = omarchy-discord-login
SOURCES += main.cpp
