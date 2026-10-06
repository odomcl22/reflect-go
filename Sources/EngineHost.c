// Own exactly one engine child. EOF on stdin means the app has gone away.
// The engine receives /dev/null, so it cannot keep the ownership pipe open.
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

static volatile sig_atomic_t stopping = 0;
static void stop_requested(int sig) { (void)sig; stopping = 1; }
static double now(void) {
    struct timespec value;
    clock_gettime(CLOCK_MONOTONIC, &value);
    return value.tv_sec + value.tv_nsec / 1e9;
}

int main(int argc, char **argv) {
    double timeout = 0;
    int first = 1;
    if (argc > 3 && strcmp(argv[1], "--timeout") == 0) {
        char *end;
        timeout = strtod(argv[2], &end);
        if (*end || !(timeout > 0 && timeout <= 3600)) return 64;
        first = 3;
    }
    if (argc <= first || argv[first][0] != '/') {
        fprintf(stderr, "Reflect Go: engine-host requires an absolute executable path.\n");
        return 64;
    }
    struct sigaction action = {0};
    action.sa_handler = stop_requested;
    sigemptyset(&action.sa_mask);
    sigaction(SIGINT, &action, NULL);
    sigaction(SIGTERM, &action, NULL);
    sigaction(SIGHUP, &action, NULL);
    sigset_t blocked, previous;
    sigemptyset(&blocked);
    sigaddset(&blocked, SIGINT); sigaddset(&blocked, SIGTERM); sigaddset(&blocked, SIGHUP);
    sigprocmask(SIG_BLOCK, &blocked, &previous);
    pid_t child = fork();
    if (child < 0) { perror("Reflect Go: fork"); return 71; }
    if (child == 0) {
        signal(SIGINT, SIG_DFL);
        signal(SIGTERM, SIG_DFL);
        signal(SIGHUP, SIG_DFL);
        sigprocmask(SIG_SETMASK, &previous, NULL);
        int input = open("/dev/null", O_RDONLY);
        if (input < 0 || dup2(input, STDIN_FILENO) < 0) _exit(71);
        if (input != STDIN_FILENO) close(input);
        execv(argv[first], &argv[first]);
        perror("Reflect Go: engine exec");
        _exit(127);
    }
    sigprocmask(SIG_SETMASK, &previous, NULL);
    // Utilities produce machine-readable stdout. Only long-lived engines log PID.
    if (!timeout) fprintf(stderr, "Reflect Go engine PID: %d\n", child);
    double began = now(), stop_at = 0;
    int timed_out = 0, killed = 0;
    for (;;) {
        int status;
        pid_t result = waitpid(child, &status, WNOHANG);
        if (result == child) {
            if (timed_out) return 124;
            return WIFEXITED(status) ? WEXITSTATUS(status) : 128 + WTERMSIG(status);
        }
        if (result < 0 && errno != EINTR) return 71;
        double current = now();
        if (timeout && current - began >= timeout) { stopping = 1; timed_out = 1; }
        if (stopping && !stop_at) {
            // No PID file or process search: this unreaped direct child is ours.
            kill(child, SIGTERM);
            stop_at = current;
        }
        if (stop_at && !killed && current - stop_at >= 8) {
            kill(child, SIGKILL);
            killed = 1;
        }
        struct pollfd owner = {STDIN_FILENO, POLLIN | POLLHUP, 0};
        if (stop_at) { poll(NULL, 0, 100); continue; }
        int count = poll(&owner, 1, 100);
        if (count > 0) {
            char bytes[64];
            if (owner.revents & (POLLERR | POLLNVAL)) stopping = 1;
            if (owner.revents & (POLLIN | POLLHUP)) {
                ssize_t size = read(STDIN_FILENO, bytes, sizeof(bytes));
                if (size == 0 || (size < 0 && errno != EINTR && errno != EAGAIN)) stopping = 1;
            }
        }
    }
}
