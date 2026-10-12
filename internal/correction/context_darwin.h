#include <stddef.h>
#include <sys/types.h>
typedef struct jt_correction_target jt_correction_target;
typedef struct jt_input_target jt_input_target;
int jt_window_capture(pid_t pid, jt_input_target **out, char **error);
int jt_window_guard(jt_input_target *target, pid_t frontPid, char **error);
// 1: editable input captured/read; 0: unsupported input; -1: explicit failure.
int jt_input_capture(pid_t pid, jt_input_target **out, char **data, char **error);
int jt_input_read(jt_input_target *target, char **text, char **error);
int jt_input_guard(jt_input_target *target, char **error);
void jt_input_release(jt_input_target *target);
int jt_correction_frontmost(pid_t *pid, char **bundle, char **error);
// 1: captured, 0: not Codex, -1: explicit failure. Strings belong to the caller.
int jt_correction_capture(pid_t pid, jt_correction_target **out, char **messages, char **error);
int jt_correction_guard(jt_correction_target *target, pid_t frontPid, char **error);
void jt_correction_release(jt_correction_target *target);
