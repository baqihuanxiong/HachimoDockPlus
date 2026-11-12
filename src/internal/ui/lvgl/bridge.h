#ifndef HACHIMODOCK_UI_BRIDGE_H
#define HACHIMODOCK_UI_BRIDGE_H
#include <stdint.h>
typedef struct { char app_id[64],action[64],text[4096]; } dock_event;
int dock_init(const char *framebuffer,const char *input,const char *font);
void dock_catalog_add(const char *id,const char *title,const char *description,const char *icon);
void dock_catalog_status(const char *status);
void dock_show_catalog(void);
void dock_emit(const char *app_id,const char *action,const char *text);
void dock_tick(void);
int dock_poll(dock_event *event);
const unsigned char *dock_pixels(void);
void dock_close(void);
#endif
