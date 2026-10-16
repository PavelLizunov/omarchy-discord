#include <QGuiApplication>
#include <QQmlApplicationEngine>
#include <QQuickWindow>
#include <QQuickItem>
#include <QMimeData>
#include <QDragEnterEvent>
#include <QDropEvent>
#include <QFile>
#include <QTemporaryDir>
#include <QTest>
#include <QDebug>
int main(int argc,char **argv){
 QGuiApplication app(argc,argv);QTemporaryDir temp;QFile source(temp.path()+"/original ü.txt");if(!source.open(QIODevice::WriteOnly))return 1;source.write("original bytes");source.close();
 QQmlApplicationEngine engine;engine.load(QUrl::fromLocalFile(QString::fromLocal8Bit(argv[1])));if(engine.rootObjects().isEmpty())return 2;
 auto root=qobject_cast<QQuickItem*>(engine.rootObjects().first());if(!root)return 3;QQuickWindow window;root->setParentItem(window.contentItem());window.resize(1040,680);window.show();QTest::qWait(100);
 auto drop=[&](QList<QUrl> urls){QMimeData mime;mime.setUrls(urls);QDragEnterEvent enter(QPointF(root->property("dropX").toReal(),root->property("dropY").toReal()),Qt::CopyAction,&mime,Qt::LeftButton,Qt::NoModifier);QGuiApplication::sendEvent(&window,&enter);if(enter.isAccepted()&&!qEnvironmentVariable("DROP_CAPTURE").isEmpty()){QTest::qWait(30);window.grabWindow().save(qEnvironmentVariable("DROP_CAPTURE"));}QDropEvent event(QPointF(root->property("dropX").toReal(),root->property("dropY").toReal()),Qt::CopyAction,&mime,Qt::LeftButton,Qt::NoModifier);QGuiApplication::sendEvent(&window,&event);return event.isAccepted();};
 if(!drop({QUrl::fromLocalFile(source.fileName())}))return 4;
 for(int i=0;i<30 && root->property("stagedCount").toInt()!=1;i++)QTest::qWait(100);
 if(root->property("stagedCount").toInt()!=1){qWarning()<<root->property("error");return 5;}
 QString copy=root->property("lastPath").toString();QFile staged(copy);if(!staged.open(QIODevice::ReadOnly)||staged.readAll()!="original bytes")return 6;staged.close();
 if(!source.exists() || root->property("sendCalls").toInt()!=0)return 7;
 if(drop({QUrl("https://example.invalid/a")}))return 8;
 root->setProperty("currentChannel",QString());QTest::qWait(10);if(drop({QUrl::fromLocalFile(source.fileName())}))return 9;
 qInfo()<<"PASS: actual ClientView Qt URL drop staged only; source intact, remote/no-channel rejected";return 0;
}
