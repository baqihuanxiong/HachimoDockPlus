//go:build lvgl && cgo
#include "bridge.h"
#include "page_picoclaw.h"
#include "ui_native.h"
#include <stdlib.h>
#include <string.h>
static const char *APP_ID="picoclaw";
static lv_obj_t *chat,*status_label,*hint_label,*history_box,*text_input,*clear_button,*send_button,*keyboard,*speech_switch,*speech_stop;
static char *last_history=NULL;
static long long last_input_revision=-1;
#define USER_MAX_W 420
#define ASSISTANT_W 552

/* ---- minimal Markdown -> spangroup rendering ---- */
typedef struct md_color {lv_style_t *style;unsigned color;struct md_color *next;} md_color;
static void md_color_event(lv_event_t *e){
    lv_obj_t *g=lv_event_get_current_target(e);
    md_color *binding=lv_obj_get_user_data(g);
    int deleting=lv_event_get_code(e)==LV_EVENT_DELETE;
    while(binding){
        md_color *next=binding->next;
        if(deleting)free(binding);
        else lv_style_set_text_color(binding->style,dock_color(dock_theme_resolve(binding->color)));
        binding=next;
    }
    if(deleting)lv_obj_set_user_data(g,NULL);
}
static void md_span(lv_obj_t *g,const char *text,unsigned color,const lv_font_t *font){
    if(!text||!*text)return;
    lv_span_t *s=lv_spangroup_new_span(g);
    lv_span_set_text(s,text);
    lv_style_t *style=lv_span_get_style(s);
    lv_style_set_text_color(style,dock_color(dock_theme_resolve(color)));
    if(font)lv_style_set_text_font(style,font);
    /* Span styles are private to LVGL's rich text, so shared object theme
       styles cannot recolor them. Keep their source colors across changes. */
    md_color *binding=malloc(sizeof(*binding));
    if(binding){
        *binding=(md_color){style,color,lv_obj_get_user_data(g)};
        lv_obj_set_user_data(g,binding);
    }
}
static void md_span_len(lv_obj_t *g,const char *text,size_t len,unsigned color,const lv_font_t *font){
    char buf[256];
    while(len>0){
        size_t n=len>sizeof(buf)-1?sizeof(buf)-1:len;
        while(n>0&&(text[n]&0xc0)==0x80)n--;
        if(n==0)n=len<sizeof(buf)-1?len:sizeof(buf)-1;
        memcpy(buf,text,n);buf[n]=0;
        md_span(g,buf,color,font);
        text+=n;len-=n;
    }
}
static void md_inline(lv_obj_t *g,const char *line,size_t len,unsigned color,const lv_font_t *font){
    int mode=0;unsigned cur_color=color;const lv_font_t *cur_font=font;
    const char *p=line,*end=line+len,*seg=line;
    while(p<end){
        int marker=0;
        if(*p=='`'&&mode!=2)marker=1;
        else if(mode!=1&&p+1<end&&p[0]=='*'&&p[1]=='*')marker=2;
        if(!marker){p++;continue;}
        if(p>seg)md_span_len(g,seg,(size_t)(p-seg),cur_color,cur_font);
        if(marker==1){
            if(mode==1){mode=0;cur_color=color;cur_font=font;}
            else if(mode==0){mode=1;cur_color=0x5eead4;}
        }else{
            if(mode==2){mode=0;cur_color=color;}
            else if(mode==0){mode=2;cur_color=0xf8fafc;}
        }
        p+=marker==1?1:2;seg=p;
    }
    if(p>seg)md_span_len(g,seg,(size_t)(p-seg),cur_color,cur_font);
}
static lv_obj_t *md_render(lv_obj_t *parent,const char *text,int width){
    lv_obj_t *g=lv_spangroup_create(parent);
    lv_obj_remove_flag(g,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);
    lv_obj_add_event_cb(g,md_color_event,LV_EVENT_STYLE_CHANGED,NULL);
    lv_obj_add_event_cb(g,md_color_event,LV_EVENT_DELETE,NULL);
    lv_obj_set_size(g,width,LV_SIZE_CONTENT);
    dock_theme_foreground(g,0xe2e8f0,0);
    lv_obj_set_style_text_line_space(g,4,0);
    int code=0,have_line=0;
    const char *p=text;
    while(*p){
        const char *nl=strchr(p,'\n');
        size_t len=nl?(size_t)(nl-p):strlen(p);
        if(len>=3&&!strncmp(p,"```",3)){code=!code;}
        else {
            if(have_line)md_span(g,"\n",0xe2e8f0,NULL);
            have_line=1;
            if(code){md_span_len(g,p,len,0x94a3b8,dock_font_small);}
            else if(len>0){
                unsigned color=0xe2e8f0;const lv_font_t *font=dock_font_body;
                const char *content=p;size_t clen=len;
                if(p[0]=='#'){
                    int level=0;while((size_t)level<len&&p[level]=='#')level++;
                    if(level>=1&&level<=3&&(size_t)level<len&&p[level]==' '){
                        font=level==1?dock_font_title:dock_font_body;color=0xf1f5f9;
                        content=p+level+1;clen=len-(size_t)level-1;
                    }
                }
                if(clen>1&&(content[0]=='-'||content[0]=='*'||content[0]=='+')&&content[1]==' '){
                    md_span(g,"• ",color,font);content+=2;clen-=2;
                }
                md_inline(g,content,clen,color,font);
            }
        }
        if(!nl)break;
        p=nl+1;
    }
    lv_spangroup_refresh(g);
    return g;
}
/* ---- chat rows ---- */
typedef struct {
    lv_obj_t *row,*header,*body;
    char *text;
    int expanded;
} pico_process_view;
static void pico_process_deleted(lv_event_t *e){
    pico_process_view *view=lv_event_get_user_data(e);
    free(view->text);free(view);
}
static void pico_process_set_expanded(pico_process_view *view,int expanded){
    view->expanded=expanded;
    lv_label_set_text(lv_obj_get_child(view->header,0),expanded?"− 处理过程 · 点击收起":"+ 处理过程 · 点击展开");
    if(expanded&&!view->body){
        /* Keep completed rows cheap: create rich-text layout only on expand.
           Plain spans preserve source newlines and wrap long process text. */
        view->body=lv_spangroup_create(view->row);
        lv_obj_remove_flag(view->body,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);
        lv_obj_add_event_cb(view->body,md_color_event,LV_EVENT_STYLE_CHANGED,NULL);
        lv_obj_add_event_cb(view->body,md_color_event,LV_EVENT_DELETE,NULL);
        lv_obj_set_pos(view->body,10,48);
        lv_obj_set_size(view->body,ASSISTANT_W-20,LV_SIZE_CONTENT);
        lv_obj_set_style_text_font(view->body,dock_font_small,0);
        lv_obj_set_style_text_line_space(view->body,4,0);
        const char *p=view->text;int first=1;
        while(*p){
            const char *nl=strchr(p,'\n');
            size_t len=nl?(size_t)(nl-p):strlen(p);
            if(!first)md_span(view->body,"\n",0x94a3b8,dock_font_small);
            first=0;
            md_span_len(view->body,p,len,0x94a3b8,dock_font_small);
            if(!nl)break;p=nl+1;
        }
        lv_spangroup_refresh(view->body);
    }
    if(view->body){
        if(expanded)lv_obj_remove_flag(view->body,LV_OBJ_FLAG_HIDDEN);
        else lv_obj_add_flag(view->body,LV_OBJ_FLAG_HIDDEN);
    }
    /* Explicitly positioned children do not dirty their parent on hide. */
    lv_obj_mark_layout_as_dirty(view->row);
}
static void pico_process_toggle(lv_event_t *e){
    pico_process_view *view=lv_event_get_user_data(e);
    int y=lv_obj_get_scroll_y(history_box);
    pico_process_set_expanded(view,!view->expanded);
    lv_obj_update_layout(history_box);
    lv_obj_scroll_to_y(history_box,y,LV_ANIM_OFF);
    lv_obj_invalidate(history_box);
}
static void pico_process_row(lv_obj_t *row,const char *text,int expanded){
    pico_process_view *view=calloc(1,sizeof(*view));
    if(!view)return;
    view->row=row;view->text=strdup(text);
    if(!view->text){free(view);return;}
    lv_obj_set_user_data(row,view);
    lv_obj_add_event_cb(row,pico_process_deleted,LV_EVENT_DELETE,view);
    view->header=dock_button(row,"处理过程",0,0,ASSISTANT_W,40,0x1e293b,NULL,NULL);
    lv_obj_add_event_cb(view->header,pico_process_toggle,LV_EVENT_CLICKED,view);
    pico_process_set_expanded(view,expanded);
}
static int line_width(const char *text,const lv_font_t *font){
    int widest=0;
    const char *p=text;
    while(*p){
        const char *nl=strchr(p,'\n');
        size_t len=nl?(size_t)(nl-p):strlen(p);
        int w=lv_txt_get_width(p,(uint32_t)len,font,0);
        if(w>widest)widest=w;
        if(!nl)break;
        p=nl+1;
    }
    return widest;
}
static void add_row_state(char role,const char *text,int expanded){
    lv_obj_t *row=lv_obj_create(history_box);
    lv_obj_remove_flag(row,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);
    lv_obj_set_width(row,lv_pct(100));
    lv_obj_set_height(row,LV_SIZE_CONTENT);
    dock_panel(row,0x121d2c);
    if(role=='u'){
        int w=line_width(text,dock_font_body);
        if(w>USER_MAX_W-28)w=USER_MAX_W-28;
        lv_obj_t *bubble=lv_obj_create(row);
        lv_obj_remove_flag(bubble,LV_OBJ_FLAG_SCROLLABLE|LV_OBJ_FLAG_CLICKABLE);
        dock_panel(bubble,0x0f766e);
        lv_obj_set_style_radius(bubble,14,0);
        lv_obj_set_style_pad_hor(bubble,14,0);lv_obj_set_style_pad_ver(bubble,9,0);
        lv_obj_set_size(bubble,w+28,LV_SIZE_CONTENT);
        lv_obj_align(bubble,LV_ALIGN_TOP_RIGHT,0,0);
        lv_obj_t *label=lv_label_create(bubble);
        lv_label_set_text(label,text);
        lv_obj_set_width(label,w);
        lv_label_set_long_mode(label,LV_LABEL_LONG_WRAP);
        lv_obj_set_style_text_font(label,dock_font_body,0);
        dock_theme_foreground(label,0xffffff,0);
        lv_obj_set_style_text_line_space(label,4,0);
    }else if(role=='p'||role=='P'){
        pico_process_row(row,text,expanded);
    }else{
        md_render(row,text,ASSISTANT_W);
    }
}
static void add_row(char role,const char *text){add_row_state(role,text,role=='p');}
static int history_record(const char **p,const char *end,char *role,const char **text,size_t *len){
    if(*p>=end||(**p!='u'&&**p!='a'&&**p!='p'&&**p!='P'))return 0;
    *role=*(*p)++;
    char *number_end;
    long n=strtol(*p,&number_end,10);
    if(number_end==*p||number_end>=end||*number_end!=':'||n<0||n>1<<20)return 0;
    *text=number_end+1;*len=(size_t)n;
    if(*len>(size_t)(end-*text))return 0;
    *p=*text+*len;
    return 1;
}
static void render_history(const char *history){
    if(!history)history="";
    if(last_history&&!strcmp(last_history,history))return;
    int follow=!last_history||lv_obj_get_scroll_bottom(history_box)<=24;
    int scroll_y=lv_obj_get_scroll_y(history_box);
    const char *p=history,*end=history+strlen(history);
    const char *old=last_history?last_history:"",*old_end=old+strlen(old);
    unsigned index=0;
    int changed=0,process_changed=0;
    char role,old_role=0;const char *text,*old_text;size_t len,old_len;
    while(history_record(&p,end,&role,&text,&len)){
        int expanded=role=='p';
        if(!changed&&history_record(&old,old_end,&old_role,&old_text,&old_len)&&
           role==old_role&&len==old_len&&!memcmp(text,old_text,len)){
            index++;continue;
        }
        /* Preserve earlier rows and their layout/font caches while the last
           assistant message grows, instead of rebuilding the whole chat. */
        if(!changed){
            if(role=='p'||role=='P'||old_role=='p'||old_role=='P')process_changed=1;
            if((role=='p'||role=='P')&&role==old_role&&lv_obj_get_child_count(history_box)>index){
                pico_process_view *view=lv_obj_get_user_data(lv_obj_get_child(history_box,index));
                if(view)expanded=view->expanded;
            }
            while(lv_obj_get_child_count(history_box)>index)lv_obj_delete(lv_obj_get_child(history_box,index));
            changed=1;
        }
        if(role=='u')follow=1;
        char *buf=malloc(len+1);
        if(!buf)break;
        memcpy(buf,text,len);buf[len]=0;
        add_row_state(role,buf,expanded);free(buf);index++;
    }
    while(lv_obj_get_child_count(history_box)>index)lv_obj_delete(lv_obj_get_child(history_box,index));
    if(!*history){
        add_row('a',"你好，我是 PicoClaw。按住 SW1 说话，或输入你想聊的内容。");
        follow=1;
    }
    free(last_history);last_history=strdup(history);
    lv_obj_update_layout(history_box);
    lv_obj_scroll_to_y(history_box,follow?LV_COORD_MAX:scroll_y,LV_ANIM_OFF);
    if(process_changed)lv_obj_invalidate(history_box);
}
/* ---- input row ---- */
static void close_keyboard(void){
    lv_obj_add_flag(keyboard,LV_OBJ_FLAG_HIDDEN);
    lv_obj_set_y(text_input,366);lv_obj_set_y(clear_button,366);lv_obj_set_y(send_button,366);
}
static void submit_input(void){
    const char *text=lv_textarea_get_text(text_input);
    if(text&&*text){dock_emit(APP_ID,"submit",text);lv_textarea_set_text(text_input,"");}
    close_keyboard();
}
static void click(lv_event_t *e) {
    const char *action=lv_event_get_user_data(e);
    if(!strcmp(action,"submit")){submit_input();return;}
    if(!strcmp(action,"input.clear")){
        lv_textarea_set_text(text_input,"");
        dock_emit(APP_ID,action,NULL);
        return;
    }
    dock_emit(APP_ID,action,NULL);
}
static void input_focused(lv_event_t *e) {
    (void)e;lv_keyboard_set_textarea(keyboard,text_input);
    lv_obj_remove_flag(keyboard,LV_OBJ_FLAG_HIDDEN);lv_obj_move_foreground(keyboard);
    lv_obj_set_y(text_input,250);lv_obj_set_y(clear_button,250);lv_obj_set_y(send_button,250);
}
static void keyboard_event(lv_event_t *e) {
    if(lv_event_get_code(e)==LV_EVENT_READY)submit_input();
    else close_keyboard();
}
static void speech_changed(lv_event_t *e){
    lv_obj_t *sw=lv_event_get_target(e);
    dock_emit(APP_ID,"speech.set",lv_obj_has_state(sw,LV_STATE_CHECKED)?"true":"false");
}

int dock_picoclaw_mount(void){
    chat=lv_obj_create(NULL);dock_panel(chat,0x0c1220);
    dock_button(chat,"返回",16,14,78,46,0x1e293b,APP_ID,"shell.back");
    dock_label(chat,"PicoClaw",112,20,148,dock_font_title,0xf1f5f9);
    dock_button(chat,"新对话",510,14,114,46,0x1e293b,APP_ID,"conversation.new");
    status_label=dock_label(chat,"准备就绪",272,27,228,dock_font_small,0x5eead4);
    lv_obj_set_height(status_label,24);lv_label_set_long_mode(status_label,LV_LABEL_LONG_DOT);
    hint_label=dock_label(chat,"按住 SW1 说话，SW2 发送",20,432,280,dock_font_small,0x94a3b8);
    lv_obj_set_height(hint_label,24);lv_label_set_long_mode(hint_label,LV_LABEL_LONG_DOT);
    dock_label(chat,"语音播报",310,432,80,dock_font_small,0xe2e8f0);
    speech_switch=dock_switch_create(chat);lv_obj_set_pos(speech_switch,390,430);lv_obj_set_size(speech_switch,48,28);
    lv_obj_add_state(speech_switch,LV_STATE_CHECKED);
    lv_obj_add_event_cb(speech_switch,speech_changed,LV_EVENT_VALUE_CHANGED,NULL);
    speech_stop=dock_button(chat,"停止播报",444,424,180,40,0x1e293b,APP_ID,"speech.stop");
    lv_obj_add_state(speech_stop,LV_STATE_DISABLED);
    history_box=lv_obj_create(chat);
    /* The default SCROLLED scrollbar style propagates style changes through
       all descendants. Use a stable scrollbar so scrolling cannot reflow text. */
    lv_obj_remove_style_all(history_box);dock_panel(history_box,0x121d2c);
    lv_obj_set_style_width(history_box,4,LV_PART_SCROLLBAR);
    lv_obj_set_style_radius(history_box,2,LV_PART_SCROLLBAR);
    lv_obj_set_style_bg_color(history_box,dock_color(0xa3a3a3),LV_PART_SCROLLBAR);
    lv_obj_set_style_bg_opa(history_box,LV_OPA_40,LV_PART_SCROLLBAR);
    lv_obj_set_pos(history_box,16,72);lv_obj_set_size(history_box,608,274);lv_obj_set_style_radius(history_box,16,0);
    lv_obj_set_scroll_dir(history_box,LV_DIR_VER);lv_obj_set_style_pad_hor(history_box,12,0);lv_obj_set_style_pad_ver(history_box,10,0);
    lv_obj_set_flex_flow(history_box,LV_FLEX_FLOW_COLUMN);lv_obj_set_style_pad_row(history_box,10,0);
    text_input=dock_textarea_create(chat);lv_obj_set_pos(text_input,16,366);lv_obj_set_size(text_input,420,48);
    lv_textarea_set_one_line(text_input,true);lv_textarea_set_max_length(text_input,1000);
    lv_textarea_set_placeholder_text(text_input,"输入消息…");lv_obj_set_style_text_font(text_input,dock_font_body,0);
    lv_obj_add_event_cb(text_input,input_focused,LV_EVENT_FOCUSED,NULL);
    clear_button=dock_button(chat,"清空",444,366,64,48,0x1e293b,NULL,NULL);
    lv_obj_add_event_cb(clear_button,click,LV_EVENT_CLICKED,"input.clear");
    send_button=dock_button(chat,"发送",514,366,110,48,0x334155,NULL,NULL);
    lv_obj_add_event_cb(send_button,click,LV_EVENT_CLICKED,"submit");
    keyboard=dock_keyboard_create(chat);lv_obj_set_height(keyboard,176);
    lv_obj_add_flag(keyboard,LV_OBJ_FLAG_HIDDEN);
    lv_obj_add_event_cb(keyboard,keyboard_event,LV_EVENT_READY,NULL);
    lv_obj_add_event_cb(keyboard,keyboard_event,LV_EVENT_CANCEL,NULL);
    free(last_history);last_history=NULL;last_input_revision=-1;
    lv_screen_load(chat);return 0;
}

void dock_picoclaw_render(const char *status,const char *partial,const char *hint,const char *history,const char *input,long long input_revision,int recording,int busy,int speech_enabled,int speaking) {
    if(!chat)return;
    if(strcmp(lv_label_get_text(status_label),status))lv_label_set_text(status_label,status);
    if(strcmp(lv_label_get_text(hint_label),hint))lv_label_set_text(hint_label,hint);
    render_history(history);
    if(recording){
        const char *current=lv_textarea_get_text(text_input);
        if(strcmp(current,partial))lv_textarea_set_text(text_input,partial);
    }else if(input_revision!=last_input_revision){
        last_input_revision=input_revision;
        lv_textarea_set_text(text_input,input?input:"");
    }
    if(busy){lv_obj_add_state(send_button,LV_STATE_DISABLED);lv_obj_add_state(clear_button,LV_STATE_DISABLED);}
    else {lv_obj_remove_state(send_button,LV_STATE_DISABLED);lv_obj_remove_state(clear_button,LV_STATE_DISABLED);}
    if(speech_enabled)lv_obj_add_state(speech_switch,LV_STATE_CHECKED);
    else lv_obj_remove_state(speech_switch,LV_STATE_CHECKED);
    if(speaking)lv_obj_remove_state(speech_stop,LV_STATE_DISABLED);
    else lv_obj_add_state(speech_stop,LV_STATE_DISABLED);
}

void dock_picoclaw_submit(void){
    if(!chat||lv_obj_has_state(send_button,LV_STATE_DISABLED))return;
    submit_input();
}

void dock_picoclaw_unmount(void){
 free(last_history);last_history=NULL;
 if(chat){lv_obj_delete(chat);chat=NULL;}
}

int dock_picoclaw_back(void){
 if(keyboard&&!lv_obj_has_flag(keyboard,LV_OBJ_FLAG_HIDDEN)){close_keyboard();return 1;}
 return 0;
}
