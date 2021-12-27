
typedef void(*NotificationListener)(void);
void go_callback(const void *cb) {
	NotificationListener nl = cb;
	nl();
}

typedef void(*NotificationParamListener)(long);
void go_callback2(const void *cb, long l) {
	NotificationParamListener nl = cb;
	nl(l);
}
