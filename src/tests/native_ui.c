/* Exercise the actual LVGL input path with a pipe in place of evdev. */
#include <assert.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/page_picoclaw.c"

static void send_input(int fd,int type,int code,int value) {
    struct input_event e={0};e.type=type;e.code=code;e.value=value;
    assert(write(fd,&e,sizeof(e))==(ssize_t)sizeof(e));
}

int main(int argc,char **argv) {
    assert(argc==2);
    assert(dock_init(NULL,NULL,argv[1])==0);
    dock_catalog_add("first","First","first test app","1");
    dock_catalog_add("second","Second","second test app","2");
    assert(lv_obj_get_child_count(catalog_list)==2);
    const uint32_t icons[]={0xf55a,0xf8a2,0xf11c,0xf00c,0xf00d,0xf053,0xf054};
    for(unsigned i=0;i<sizeof(icons)/sizeof(icons[0]);i++) {
        lv_font_glyph_dsc_t glyph={0};
        assert(lv_font_get_glyph_dsc(dock_font_body,&glyph,icons[i],0));
        assert(!glyph.is_placeholder&&glyph.resolved_font==&lv_font_montserrat_22);
        lv_font_glyph_release_draw_data(&glyph);
    }
    lv_display_set_resolution(display,480,640);
    lv_display_set_rotation(display,LV_DISPLAY_ROTATION_270);
    lv_obj_update_layout(catalog);
    assert(lv_display_get_horizontal_resolution(display)==640);
    assert(lv_display_get_vertical_resolution(display)==480);

    int fds[2];assert(pipe(fds)==0);
    touch_fd=fds[0];fcntl(touch_fd,F_SETFL,O_NONBLOCK);
    touch_device=lv_indev_create();
    lv_indev_set_type(touch_device,LV_INDEV_TYPE_POINTER);
    lv_indev_set_read_cb(touch_device,touch_read);
    lv_indev_set_display(touch_device,display);

    /* A nonzero tracking ID in slot 7, without BTN_TOUCH, must click the
       catalog button after native (179,320) -> landscape (320,300). */
    send_input(fds[1],EV_ABS,ABS_MT_SLOT,7);
    send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,19);
    send_input(fds[1],EV_ABS,ABS_MT_POSITION_X,179);
    send_input(fds[1],EV_ABS,ABS_MT_POSITION_Y,320);
    send_input(fds[1],EV_SYN,SYN_REPORT,0);
    lv_indev_read(touch_device);
    lv_point_t point;lv_indev_get_point(touch_device,&point);
    assert(point.x==320&&point.y==300);
    send_input(fds[1],EV_ABS,ABS_MT_TRACKING_ID,-1);
    send_input(fds[1],EV_SYN,SYN_REPORT,0);
    lv_indev_read(touch_device);
    dock_event event;assert(dock_poll(&event)==1&&!strcmp(event.action,"shell.open")&&!strcmp(event.text,"first"));
    assert(dock_poll(&event)==0);
    lv_obj_t *second=lv_obj_get_child(catalog_list,1);
    lv_obj_send_event(lv_obj_get_child(second,3),LV_EVENT_CLICKED,NULL);
    assert(dock_poll(&event)==1&&!strcmp(event.text,"second")&&!strcmp(event.action,"shell.open"));
    for(int i=0;i<3;i++) {
        assert(dock_picoclaw_mount()==0);
        dock_picoclaw_render("就绪","","","开始说话",0,"未接入音频设备",0);
        lv_textarea_set_text(text_input,"hello");
        lv_obj_send_event(send_button,LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.app_id,"picoclaw")&&!strcmp(event.action,"submit")&&!strcmp(event.text,"hello"));
        lv_obj_send_event(lv_obj_get_child(chat,0),LV_EVENT_CLICKED,NULL);
        assert(dock_poll(&event)==1&&!strcmp(event.app_id,"picoclaw")&&!strcmp(event.action,"shell.back"));
        dock_show_catalog();dock_picoclaw_unmount();
    }
    close(fds[1]);dock_close();
    puts("LVGL: keyboard glyph fallback, landscape touch mapping and nonzero tracking ID click passed");
    return 0;
}
