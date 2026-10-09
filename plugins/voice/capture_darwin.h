#pragma once
#include <stddef.h>
#include <stdint.h>
typedef struct jt_capture jt_capture_t;
typedef struct {
    int64_t select_ms, configure_ms, start_ms, first_buffer_ms;
} jt_capture_start_report_t;
void *jt_audio_activity_begin(void);
void jt_audio_activity_end(void *token);
int jt_capture_start(jt_capture_t **capture, jt_capture_start_report_t *report,
                     char *error, size_t error_size);
int jt_capture_read(jt_capture_t *capture, void *data, size_t size,
                    char *error, size_t error_size);
void jt_capture_stop(jt_capture_t *capture);
void jt_capture_destroy(jt_capture_t *capture);
