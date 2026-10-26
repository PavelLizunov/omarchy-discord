#include <QApplication>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QLocalSocket>
#include <QLockFile>
#include <QStandardPaths>
#include <QTimer>
#include <QVBoxLayout>
#include <QWebEngineDownloadRequest>
#include <QWebEnginePage>
#include <QWebEnginePermission>
#include <QWebEngineProfile>
#include <QWebEngineSettings>
#include <QWebEngineUrlRequestInfo>
#include <QWebEngineUrlRequestInterceptor>
#include <QWebEngineView>
#include <QWidget>
#include <functional>

class LoginPage : public QWebEnginePage {
public:
    using QWebEnginePage::QWebEnginePage;
protected:
    bool acceptNavigationRequest(const QUrl &url, NavigationType, bool mainFrame) override {
        return !mainFrame || (url.scheme() == "https" && url.host() == "discord.com");
    }
    void javaScriptConsoleMessage(JavaScriptConsoleMessageLevel, const QString &,
                                  int, const QString &) override {}
};

class SessionCapture : public QWebEngineUrlRequestInterceptor {
public:
    using QWebEngineUrlRequestInterceptor::QWebEngineUrlRequestInterceptor;
    std::function<void(QByteArray)> receive;
    void interceptRequest(QWebEngineUrlRequestInfo &info) override {
        const auto url = info.requestUrl();
        if (url.scheme() != "https" || url.host() != "discord.com"
            || !url.path().startsWith("/api/")
            || info.firstPartyUrl().host() != "discord.com") return;
        const auto headers = info.httpHeaders();
        for (auto it = headers.cbegin(); it != headers.cend(); ++it) {
            if (it.key().compare("authorization", Qt::CaseInsensitive) != 0) continue;
            const auto token = it.value().trimmed();
            if (token.size() >= 30 && token.size() <= 4096 && !token.contains(' '))
                receive(token);
            break;
        }
    }
};

int main(int argc, char **argv) {
    QApplication app(argc, argv);
    app.setApplicationName("omarchy-discord-login");
    app.setDesktopFileName("omarchy-discord-login");
    const auto runtime = QStandardPaths::writableLocation(QStandardPaths::RuntimeLocation);
    if (runtime.isEmpty()) return 2;
    QLockFile lock(runtime + "/omarchy-discord-login.lock");
    if (!lock.tryLock(0)) return 3;

    QWidget window;
    window.setWindowTitle(QObject::tr("Discord sign-in — discord.com"));
    window.resize(1040, 820);
    QVBoxLayout layout(&window);
    QLabel status(QObject::tr("Sign in at https://discord.com using email and password or QR. "
        "Complete any CAPTCHA or two-factor check here. This window shares the resulting "
        "session with Omacord; it does not save your password or browser profile."));
    status.setTextFormat(Qt::PlainText);
    status.setWordWrap(true);
    status.setMargin(8);
    layout.addWidget(&status);

    // An unnamed profile is off-the-record and never reads an existing browser session.
    QWebEngineProfile profile;
    profile.setHttpCacheType(QWebEngineProfile::MemoryHttpCache);
    profile.setPersistentCookiesPolicy(QWebEngineProfile::NoPersistentCookies);
    SessionCapture capture;
    profile.setUrlRequestInterceptor(&capture);
    QWebEngineView view;
    auto *page = new LoginPage(&profile, &view);
    view.setPage(page);
    view.settings()->setAttribute(QWebEngineSettings::FullScreenSupportEnabled, false);
    QObject::connect(page, &QWebEnginePage::permissionRequested, &window,
                     [](QWebEnginePermission permission) { permission.deny(); });
    QObject::connect(&profile, &QWebEngineProfile::downloadRequested, &window,
                     [](QWebEngineDownloadRequest *request) { request->cancel(); });
    layout.addWidget(&view, 1);

    QLocalSocket socket;
    QTimer deadline;
    deadline.setSingleShot(true);
    bool submitted = false;
    bool completed = false;
    bool failed = false;
    QByteArray incoming;
    auto fail = [&](const QString &message) {
        completed = true;
        failed = true;
        deadline.stop();
        socket.abort();
        status.setText(message);
        view.setEnabled(false);
    };
    QObject::connect(&deadline, &QTimer::timeout, &window, [&] {
        fail(QObject::tr("Sign-in timed out. Close this window and try again."));
    });
    QObject::connect(&socket, &QLocalSocket::errorOccurred, &window, [&] {
        if (!completed) fail(QObject::tr("Cannot reach the Omacord backend. "
            "Close this window, start the backend and try again."));
    });
    QObject::connect(&socket, &QLocalSocket::disconnected, &window, [&] {
        if (!completed) fail(QObject::tr("The Omacord backend disconnected. "
            "Close this window and try again."));
    });
    QObject::connect(&socket, &QLocalSocket::readyRead, &window, [&] {
        incoming += socket.readAll();
        if (incoming.size() > 16 * 1024 * 1024) {
            fail(QObject::tr("The backend response was too large. Close this window and try again."));
            return;
        }
        while (incoming.contains('\n')) {
            const auto end = incoming.indexOf('\n');
            const auto message = QJsonDocument::fromJson(incoming.left(end)).object();
            incoming.remove(0, end + 1);
            if (message.value("type") != "response" || message.value("id").toInt() != 1) continue;
            if (!message.value("ok").toBool()) {
                // Never reflect backend payloads into this credential-entry window.
                fail(QObject::tr("Discord signed in, but Omacord could not accept the session. "
                    "Close this window and try again."));
                return;
            }
            completed = true;
            deadline.stop();
            const bool stored = message.value("result").toObject().value("keyring_stored").toBool();
            if (stored) {
                app.exit(0);
            } else {
                status.setText(QObject::tr("Signed in, but the session could not be saved to the "
                    "system keyring. You will need to sign in again after a backend restart. "
                    "You can close this window."));
                view.hide();
            }
            return;
        }
    });
    capture.receive = [&](QByteArray token) {
        // The interceptor runs on the network thread; all socket/UI work stays on the GUI thread.
        QMetaObject::invokeMethod(&window, [&, token = std::move(token)]() mutable {
            if (submitted || completed) { token.fill('\0'); return; }
            if (socket.state() != QLocalSocket::ConnectedState) {
                token.fill('\0');
                fail(QObject::tr("The backend is not ready. Close this window and try again."));
                return;
            }
            submitted = true;
            status.setText(QObject::tr("Discord signed in. Connecting Omacord…"));
            QJsonObject command{{"v", 1}, {"id", 1}, {"command", "login"},
                                {"token", QString::fromUtf8(token)}};
            auto payload = QJsonDocument(command).toJson(QJsonDocument::Compact) + '\n';
            socket.write(payload);
            socket.flush();
            payload.fill('\0');
            token.fill('\0');
            command.remove("token");
            view.hide();
            deadline.start(45000);
        }, Qt::QueuedConnection);
    };
    QObject::connect(&view, &QWebEngineView::loadFinished, &window, [&](bool ok) {
        if (!ok && !submitted && !completed)
            status.setText(QObject::tr("Discord could not load. Check your connection and "
                "close this window to retry."));
    });
    QObject::connect(&socket, &QLocalSocket::connected, &window, [&] {
        view.load(QUrl("https://discord.com/login"));
    });
    socket.connectToServer(runtime + "/omarchy-discord/backend.sock");
    deadline.start(10 * 60 * 1000);
    window.show();
    const int result = app.exec();
    profile.setUrlRequestInterceptor(nullptr);
    return failed ? 2 : (completed ? result : 1);
}
