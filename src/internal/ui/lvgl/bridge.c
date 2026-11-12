//go:build lvgl && cgo

#include "bridge.h"
#include "ui_native.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <fcntl.h>
#include <unistd.h>
#include <sys/ioctl.h>
#include <linux/kd.h>
#include <linux/input.h>
#include <stdbool.h>
#include <errno.h>

#define W 640
#define H 480
static lv_obj_t *catalog,*catalog_list,*catalog_status;
lv_font_t *dock_font_body,*dock_font_title,*dock_font_small;
static lv_display_t *display;
static unsigned char *pixels;
static void *draw_buffer;
static int tty_fd=-1;
static int touch_fd=-1,touch_slot;
static lv_indev_t *touch_device;
static struct { int x,y; bool down; } touch_slots[16];
static lv_point_t last_touch;
static bool touch_released=true;
static dock_event events[16];
static unsigned event_read,event_write;

static bool is_touch_device(int fd) {
    struct input_absinfo x,y;
    return ioctl(fd,EVIOCGABS(ABS_MT_POSITION_X),&x)==0 &&
           ioctl(fd,EVIOCGABS(ABS_MT_POSITION_Y),&y)==0 && x.maximum>x.minimum && y.maximum>y.minimum;
}
static int open_touch(const char *path) {
    if(path&&*path&&strcmp(path,"auto")) {
        int fd=open(path,O_RDONLY|O_NONBLOCK);
        if(fd>=0&&is_touch_device(fd))return fd;
        if(fd>=0)close(fd);
    }
    for(int i=0;i<32;i++) {
        char name[64];snprintf(name,sizeof(name),"/dev/input/event%d",i);
        int fd=open(name,O_RDONLY|O_NONBLOCK);
        if(fd>=0&&is_touch_device(fd))return fd;
        if(fd>=0)close(fd);
    }
    return -1;
}
static void touch_read(lv_indev_t *device,lv_indev_data_t *data) {
    (void)device;
    data->continue_reading=false;
    if(touch_fd<0){data->state=LV_INDEV_STATE_RELEASED;return;}
    struct input_event event;
    while(read(touch_fd,&event,sizeof(event))==(ssize_t)sizeof(event)) {
        if(event.type==EV_ABS){
            if(event.code==ABS_MT_SLOT)touch_slot=event.value>=0&&event.value<16?event.value:15;
            else if(event.code==ABS_MT_TRACKING_ID)touch_slots[touch_slot].down=event.value>=0;
            else if(event.code==ABS_MT_POSITION_X)touch_slots[touch_slot].x=event.value;
            else if(event.code==ABS_MT_POSITION_Y)touch_slots[touch_slot].y=event.value;
        }else if(event.type==EV_SYN&&event.code==SYN_DROPPED){
            memset(touch_slots,0,sizeof(touch_slots));touch_released=true;
        }else if(event.type==EV_SYN&&event.code==SYN_REPORT){
            touch_released=true;
            for(int i=0;i<16;i++)if(touch_slots[i].down){last_touch.x=touch_slots[i].x;last_touch.y=touch_slots[i].y;touch_released=false;break;}
            data->continue_reading=true;
            break;
        }
    }
    data->point=last_touch;
    data->state=touch_released?LV_INDEV_STATE_RELEASED:LV_INDEV_STATE_PRESSED;
}

lv_color_t dock_color(unsigned rgb) { return lv_color_hex(rgb); }
static uint32_t tick_ms(void) {
    struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);
    return (uint32_t)((uint64_t)t.tv_sec*1000+t.tv_nsec/1000000);
}
void dock_emit(const char *app_id,const char *action,const char *text) {
    if(event_write-event_read>=16)return;
    dock_event *e=&events[event_write++%16];
    snprintf(e->app_id,sizeof(e->app_id),"%s",app_id?app_id:"");
    snprintf(e->action,sizeof(e->action),"%s",action);
    snprintf(e->text,sizeof(e->text),"%s",text?text:"");
}
lv_obj_t *dock_label(lv_obj_t *parent,const char *text,int x,int y,int width,const lv_font_t *font,unsigned fg) {
    lv_obj_t *o=lv_label_create(parent);lv_label_set_text(o,text);
    lv_obj_set_pos(o,x,y);lv_obj_set_width(o,width);
    lv_obj_set_style_text_font(o,font,0);lv_obj_set_style_text_color(o,dock_color(fg),0);
    lv_label_set_long_mode(o,LV_LABEL_LONG_WRAP);return o;
}
void dock_panel(lv_obj_t *o,unsigned bg) {
    lv_obj_set_style_bg_color(o,dock_color(bg),0);lv_obj_set_style_bg_opa(o,LV_OPA_COVER,0);
    lv_obj_set_style_border_width(o,0,0);lv_obj_set_style_pad_all(o,0,0);
    lv_obj_set_style_radius(o,0,0);
}
typedef struct {char app_id[64],action[64];} action_binding;
static void bound_click(lv_event_t *e) {
 action_binding *b=lv_event_get_user_data(e);dock_emit(b->app_id,b->action,NULL);
}
static void free_binding(lv_event_t *e){free(lv_event_get_user_data(e));}
lv_obj_t *dock_button(lv_obj_t *parent,const char *text,int x,int y,int width,int height,unsigned bg,const char *app_id,const char *action) {
 lv_obj_t *b=lv_button_create(parent);lv_obj_set_pos(b,x,y);lv_obj_set_size(b,width,height);
 lv_obj_set_style_bg_color(b,dock_color(bg),0);lv_obj_set_style_radius(b,14,0);
 lv_obj_set_style_shadow_width(b,0,0);lv_obj_set_style_border_width(b,0,0);
 lv_obj_t *t=dock_label(b,text,0,0,width-8,dock_font_body,0xffffff);lv_obj_set_style_text_align(t,LV_TEXT_ALIGN_CENTER,0);lv_obj_center(t);
 if(action){
  action_binding *binding=calloc(1,sizeof(*binding));LV_ASSERT_MALLOC(binding);
  snprintf(binding->app_id,sizeof(binding->app_id),"%s",app_id?app_id:"");
  snprintf(binding->action,sizeof(binding->action),"%s",action);
  lv_obj_add_event_cb(b,bound_click,LV_EVENT_CLICKED,binding);
  lv_obj_add_event_cb(b,free_binding,LV_EVENT_DELETE,binding);
 }
 return b;
}
static void preview_flush(lv_display_t *disp,const lv_area_t *area,uint8_t *data) {
    int width=area->x2-area->x1+1;
    for(int y=area->y1;y<=area->y2;y++)
        memcpy(pixels+((size_t)y*W+area->x1)*4,data+(size_t)(y-area->y1)*width*4,(size_t)width*4);
    lv_display_flush_ready(disp);
}

static void dock_catalog_open(lv_event_t *e){action_binding *b=lv_event_get_user_data(e);dock_emit("","shell.open",b->action);}

int dock_init(const char *framebuffer,const char *input,const char *font) {
    lv_init();lv_tick_set_cb(tick_ms);
    if(framebuffer&&*framebuffer){
        int fd=open(framebuffer,O_RDWR);if(fd<0)return -1;close(fd);
        display=lv_linux_fbdev_create();lv_linux_fbdev_set_file(display,framebuffer);
        if(lv_display_get_horizontal_resolution(display)!=480||lv_display_get_vertical_resolution(display)!=640)return -2;
        lv_display_set_rotation(display,LV_DISPLAY_ROTATION_270);
        tty_fd=open("/dev/tty0",O_RDWR);if(tty_fd>=0)ioctl(tty_fd,KDSETMODE,KD_GRAPHICS);
        touch_fd=open_touch(input);
        if(touch_fd>=0){touch_device=lv_indev_create();lv_indev_set_type(touch_device,LV_INDEV_TYPE_POINTER);lv_indev_set_read_cb(touch_device,touch_read);lv_indev_set_display(touch_device,display);}
    }else{
        pixels=calloc(W*H,4);draw_buffer=malloc(W*64*4);
        if(!pixels||!draw_buffer)return -4;
        display=lv_display_create(W,H);
        lv_display_set_color_format(display,LV_COLOR_FORMAT_XRGB8888);
        lv_display_set_buffers(display,draw_buffer,NULL,W*64*4,LV_DISPLAY_RENDER_MODE_PARTIAL);
        lv_display_set_flush_cb(display,preview_flush);
    }
    char path[1024];snprintf(path,sizeof(path),"A:%s",font);
    dock_font_body=lv_tiny_ttf_create_file(path,22);
    dock_font_title=lv_tiny_ttf_create_file(path,30);
    dock_font_small=lv_tiny_ttf_create_file(path,18);
    if(!dock_font_body||!dock_font_title||!dock_font_small)return -5;
    /* The Chinese font lacks LVGL's private-use keyboard icon glyphs. */
    dock_font_body->fallback=&lv_font_montserrat_22;
    dock_font_title->fallback=&lv_font_montserrat_22;
    dock_font_small->fallback=&lv_font_montserrat_22;
    lv_theme_t *theme=lv_theme_default_init(display,dock_color(0x14b8a6),dock_color(0x6366f1),true,dock_font_body);
    lv_display_set_theme(display,theme);

    catalog=lv_obj_create(NULL);dock_panel(catalog,0x0c1220);
    dock_label(catalog,"HachimoDock",24,24,592,dock_font_title,0xf1f5f9);
    dock_label(catalog,"应用",26,76,580,dock_font_body,0x94a3b8);
    catalog_list=lv_obj_create(catalog);dock_panel(catalog_list,0x0c1220);
    lv_obj_set_pos(catalog_list,20,120);lv_obj_set_size(catalog_list,600,302);
    lv_obj_set_scroll_dir(catalog_list,LV_DIR_VER);
    lv_obj_set_flex_flow(catalog_list,LV_FLEX_FLOW_COLUMN);
    lv_obj_set_style_pad_row(catalog_list,16,0);
    catalog_status=dock_label(catalog,"选择一个应用开始",26,438,588,dock_font_small,0x94a3b8);
    lv_obj_set_height(catalog_status,26);lv_label_set_long_mode(catalog_status,LV_LABEL_LONG_DOT);
    if(framebuffer&&*framebuffer&&touch_fd<0)dock_label(catalog,"未检测到触摸设备",382,78,236,dock_font_small,0xfbbf24);
    lv_screen_load(catalog);return 0;
}
void dock_catalog_add(const char *id,const char *title,const char *description,const char *icon) {
 lv_obj_t *card=lv_obj_create(catalog_list);dock_panel(card,0x172333);
 lv_obj_set_size(card,600,224);lv_obj_set_style_radius(card,22,0);
 lv_obj_remove_flag(card,LV_OBJ_FLAG_SCROLLABLE);
 lv_obj_t *badge=lv_obj_create(card);dock_panel(badge,0x134e4a);lv_obj_set_size(badge,56,56);lv_obj_set_pos(badge,22,22);lv_obj_set_style_radius(badge,16,0);
 dock_label(badge,icon,16,7,36,dock_font_title,0x5eead4);
 dock_label(card,title,94,26,470,dock_font_title,0xf8fafc);
 dock_label(card,description,24,100,550,dock_font_body,0xcbd5e1);
 lv_obj_t *open=dock_button(card,"打开",22,154,556,52,0x0f766e,NULL,NULL);
 action_binding *binding=calloc(1,sizeof(*binding));LV_ASSERT_MALLOC(binding);
 snprintf(binding->action,sizeof(binding->action),"%s",id);
 // The binding owns the ID for the entire card lifetime.
 lv_obj_add_event_cb(open,dock_catalog_open,LV_EVENT_CLICKED,binding);
 lv_obj_add_event_cb(open,free_binding,LV_EVENT_DELETE,binding);
}
void dock_catalog_status(const char *status){lv_label_set_text(catalog_status,*status?status:"选择一个应用开始");}
void dock_show_catalog(void){lv_screen_load(catalog);}
void dock_tick(void){lv_timer_handler();}
int dock_poll(dock_event *event){if(event_read==event_write)return 0;*event=events[event_read++%16];return 1;}
const unsigned char *dock_pixels(void){return pixels;}
void dock_close(void){
 if(catalog){dock_show_catalog();lv_obj_clean(catalog);}
 if(dock_font_body)lv_tiny_ttf_destroy(dock_font_body);
 if(dock_font_title)lv_tiny_ttf_destroy(dock_font_title);
 if(dock_font_small)lv_tiny_ttf_destroy(dock_font_small);
 dock_font_body=dock_font_title=dock_font_small=NULL;
 lv_deinit();
 free(pixels);free(draw_buffer);pixels=NULL;draw_buffer=NULL;
 if(tty_fd>=0){ioctl(tty_fd,KDSETMODE,KD_TEXT);close(tty_fd);tty_fd=-1;}
 if(touch_fd>=0){close(touch_fd);touch_fd=-1;}
}
