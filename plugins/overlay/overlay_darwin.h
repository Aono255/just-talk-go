#pragma once

void jt_overlay_helper_init(const char *position, double scale);
void jt_overlay_helper_run_app(void);
void jt_overlay_helper_configure(int show_text);
void jt_overlay_helper_show(const char *state, const char *label, unsigned short r, unsigned short g, unsigned short b, const char *text, double level, const char *detail);
void jt_overlay_helper_hide(void);
void jt_overlay_helper_close(void);
