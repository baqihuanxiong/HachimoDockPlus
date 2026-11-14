#ifndef HACHIMODOCK_UI_BRIDGE_H
#define HACHIMODOCK_UI_BRIDGE_H
#include <stdint.h>
typedef struct { char app_id[64],action[64],text[4096]; } dock_event;
int dock_init(const char *framebuffer,const char *input,const char *font);
void dock_catalog_add(const char *id,const char *title,const char *description,const char *icon);
int dock_catalog_image(const char *id,const unsigned char *pixels,int width,int height);
void dock_catalog_image_fill(const char *id);
void dock_catalog_status(const char *status);
void dock_catalog_running(const char *id,int running);
void dock_theme_set(unsigned rgb);
void dock_theme_appearance(unsigned rgb,int light);
/* battery=-1 unknown, 0..100 estimated; wifi=0 disconnected, 1..4 signal;
   bluetooth=0 hidden, 1 generic, 2 headphones, 3 speaker, 4 keyboard, 5 mouse,
   6 gamepad, 7 phone, 8 computer, 9 watch, 10 camera. */
void dock_catalog_header(const char *clock,int battery,int charging,int wifi,int bluetooth);
void dock_show_catalog(void);
int dock_catalog_move(int dx,int dy);
int dock_catalog_activate(void);
void dock_screensaver_show(const char *kind);
void dock_screensaver_time(const char *clock,const char *date);
void dock_emit(const char *app_id,const char *action,const char *text);
void dock_tick(void);
int dock_poll(dock_event *event);
const unsigned char *dock_pixels(void);
void dock_close(void);
#endif
