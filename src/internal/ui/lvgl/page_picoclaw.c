//go:build lvgl && cgo
#include "bridge.h"
#include "page_picoclaw.h"
#include "ui_native.h"
#include <string.h>
static const char *APP_ID="picoclaw";
static lv_obj_t *chat,*status_label,*partial_label,*history_label,*history_box;
static lv_obj_t *voice_button,*voice_text,*audio_label,*text_input,*send_button,*keyboard;
static void click(lv_event_t *e) {
    const char *action=lv_event_get_user_data(e);
    if(!strcmp(action,"submit")){
        const char *text=lv_textarea_get_text(text_input);
        if(text&&*text){dock_emit(APP_ID,action,text);lv_textarea_set_text(text_input,"");}
        lv_obj_add_flag(keyboard,LV_OBJ_FLAG_HIDDEN);
        lv_obj_set_y(text_input,366);lv_obj_set_y(send_button,366);
    }else dock_emit(APP_ID,action,NULL);
}
static void input_focused(lv_event_t *e) {
    (void)e;lv_keyboard_set_textarea(keyboard,text_input);
    lv_obj_remove_flag(keyboard,LV_OBJ_FLAG_HIDDEN);lv_obj_move_foreground(keyboard);
    lv_obj_set_y(text_input,250);lv_obj_set_y(send_button,250);
}
static void keyboard_event(lv_event_t *e) {
    if(lv_event_get_code(e)==LV_EVENT_READY){
        const char *text=lv_textarea_get_text(text_input);
        if(text&&*text){dock_emit(APP_ID,"submit",text);lv_textarea_set_text(text_input,"");}
    }
    lv_obj_add_flag(keyboard,LV_OBJ_FLAG_HIDDEN);
    lv_obj_set_y(text_input,366);lv_obj_set_y(send_button,366);
}

int dock_picoclaw_mount(void){
    chat=lv_obj_create(NULL);dock_panel(chat,0x0c1220);
    dock_button(chat,"返回",16,14,78,46,0x1e293b,APP_ID,"shell.back");
    dock_label(chat,"PicoClaw",112,20,390,dock_font_title,0xf1f5f9);
    dock_button(chat,"新对话",510,14,114,46,0x1e293b,APP_ID,"conversation.new");
    status_label=dock_label(chat,"准备就绪",20,76,600,dock_font_body,0x5eead4);
    lv_obj_set_height(status_label,28);lv_label_set_long_mode(status_label,LV_LABEL_LONG_DOT);
    partial_label=dock_label(chat,"",20,107,600,dock_font_small,0x94a3b8);
    lv_obj_set_height(partial_label,24);lv_label_set_long_mode(partial_label,LV_LABEL_LONG_DOT);
    history_box=lv_obj_create(chat);dock_panel(history_box,0x121d2c);
    lv_obj_set_pos(history_box,16,138);lv_obj_set_size(history_box,608,212);lv_obj_set_style_radius(history_box,16,0);
    lv_obj_set_scroll_dir(history_box,LV_DIR_VER);lv_obj_set_style_pad_all(history_box,18,0);
    history_label=dock_label(history_box,"你好，我是 PicoClaw。\n\n说句话，或输入你想聊的内容。",0,0,570,dock_font_body,0xe2e8f0);
    lv_obj_set_style_text_line_space(history_label,5,0);
    text_input=lv_textarea_create(chat);lv_obj_set_pos(text_input,16,366);lv_obj_set_size(text_input,482,48);
    lv_textarea_set_one_line(text_input,true);lv_textarea_set_max_length(text_input,1000);
    lv_textarea_set_placeholder_text(text_input,"输入消息…");lv_obj_set_style_text_font(text_input,dock_font_body,0);
    lv_obj_add_event_cb(text_input,input_focused,LV_EVENT_FOCUSED,NULL);
    send_button=dock_button(chat,"发送",510,366,114,48,0x334155,NULL,NULL);
    lv_obj_add_event_cb(send_button,click,LV_EVENT_CLICKED,"submit");
    voice_button=dock_button(chat,"开始说话",16,430,360,44,0x0f766e,APP_ID,"voice.toggle");
    voice_text=lv_obj_get_child(voice_button,0);
    audio_label=dock_label(chat,"",392,440,232,dock_font_small,0x94a3b8);lv_obj_set_height(audio_label,25);lv_label_set_long_mode(audio_label,LV_LABEL_LONG_DOT);
    keyboard=lv_keyboard_create(chat);lv_obj_set_height(keyboard,176);
    lv_obj_add_flag(keyboard,LV_OBJ_FLAG_HIDDEN);
    lv_obj_add_event_cb(keyboard,keyboard_event,LV_EVENT_READY,NULL);
    lv_obj_add_event_cb(keyboard,keyboard_event,LV_EVENT_CANCEL,NULL);
    lv_screen_load(chat);return 0;
}

void dock_picoclaw_render(const char *status,const char *partial,const char *history,const char *voice_label,int voice_enabled,const char *audio_status,int busy) {
    if(strcmp(lv_label_get_text(status_label),status))lv_label_set_text(status_label,status);
    if(strcmp(lv_label_get_text(partial_label),partial))lv_label_set_text(partial_label,partial);
    if(*history&&strcmp(lv_label_get_text(history_label),history)){
        lv_label_set_text(history_label,history);lv_obj_update_layout(history_box);lv_obj_scroll_to_y(history_box,LV_COORD_MAX,LV_ANIM_OFF);
    }
    if(!*history)lv_label_set_text(history_label,"你好，我是 PicoClaw。\n\n说句话，或输入你想聊的内容。");
    lv_label_set_text(voice_text,voice_label);
    if(voice_enabled)lv_obj_remove_state(voice_button,LV_STATE_DISABLED);else lv_obj_add_state(voice_button,LV_STATE_DISABLED);
    lv_label_set_text(audio_label,audio_status);
    if(busy){lv_obj_add_state(send_button,LV_STATE_DISABLED);lv_obj_add_state(text_input,LV_STATE_DISABLED);}
    else {lv_obj_remove_state(send_button,LV_STATE_DISABLED);lv_obj_remove_state(text_input,LV_STATE_DISABLED);}
}

void dock_picoclaw_unmount(void){
 if(chat){lv_obj_delete(chat);chat=NULL;}
}
