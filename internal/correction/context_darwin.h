#include <stddef.h>
#include <sys/types.h>
typedef struct jt_correction_target jt_correction_target;
int jt_correction_frontmost(pid_t *pid, char **bundle, char **error);
// 1: captured, 0: not Codex, -1: explicit failure. Strings belong to the caller.
int jt_correction_capture(pid_t pid, jt_correction_target **out, char **messages, char **error);
int jt_correction_guard(jt_correction_target *target, pid_t frontPid, char **error);
int jt_correction_submitted_after(jt_correction_target *current, jt_correction_target *previous, char **text);
void jt_correction_release(jt_correction_target *target);
