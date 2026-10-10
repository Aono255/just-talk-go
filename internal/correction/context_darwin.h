#include <stddef.h>
typedef struct jt_correction_target jt_correction_target;
// 1: captured, 0: not Codex, -1: explicit failure. Strings belong to the caller.
int jt_correction_capture(jt_correction_target **out, char **messages, char **error);
int jt_correction_guard(jt_correction_target *target, char **error);
void jt_correction_release(jt_correction_target *target);
