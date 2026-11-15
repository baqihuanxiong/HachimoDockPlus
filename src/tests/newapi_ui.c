/* Real LVGL: a single-page dashboard, scrollable rankings, themes and reset controls. */
#include <assert.h>
#include <time.h>
#include "../internal/ui/lvgl/bridge.c"
#include "../internal/ui/lvgl/statusbar.c"
#include "../internal/ui/lvgl/theme.c"
#include "../internal/ui/lvgl/catalog.c"
#include "../internal/ui/lvgl/screensaver.c"
#include "../internal/ui/lvgl/page_newapi.c"
static void newapi_screenshot(const char *path){lv_refr_now(display);const unsigned char *p=dock_pixels();FILE *f=fopen(path,"wb");assert(f);fprintf(f,"P6\n640 480\n255\n");for(int i=0;i<640*480;i++){fputc(p[i*4+2],f);fputc(p[i*4+1],f);fputc(p[i*4],f);}fclose(f);}
static int newapi_compare(const void *a,const void *b){double x=*(const double*)a,y=*(const double*)b;return (x>y)-(x<y);}
static void newapi_fixture(void){
 dock_newapi_render("最近更新 11-15 22:50:00",0,0,"");
 dock_newapi_metric(0,"$0.05","12","100%");dock_newapi_metric(1,"$28.38","1.3k","99.8%");dock_newapi_metric(2,"$153.60","6.2k","99.2%");
 dock_newapi_balance(0,"阿里千问","75.0%","月度剩余额度");dock_newapi_balance(1,"小米 MiMo","42.97B","Credits 剩余 · 87.3%");
 dock_newapi_rows(0,10);dock_newapi_rows(1,10);
 const double shares[]={50,20,10,8,4,3,2,1.5,1,0.5};
 const char *models[]={"qwen3-coder-plus","mimo-v2.5-pro","qwen3.6-plus","deepseek-v4","glm-5.1"};
 for(int i=0;i<10;i++){char name[80],value[32];snprintf(name,sizeof(name),"%s",models[i%5]);snprintf(value,sizeof(value),"%.1f%%",shares[i]);dock_newapi_row(0,i,name,value,(int)(shares[i]*10));snprintf(name,sizeof(name),"user-%d",i+1);snprintf(value,sizeof(value),"$%.2f",24.6/(i+1));dock_newapi_row(1,i,name,value,1000/(i+1));}
}
int main(int argc,char **argv){
 assert(argc==2);assert(dock_init(NULL,NULL,argv[1])==0);dock_event event;
 for(int pass=0;pass<3;pass++){
  assert(dock_newapi_mount()==0);newapi_fixture();lv_obj_update_layout(newapi_screen);
  assert(!strcmp(lv_label_get_text(newapi_metrics[1][0]),"$28.38"));assert(newapi_counts[0]==10&&newapi_counts[1]==10);assert(lv_bar_get_value(newapi_bars[0][0])==500);
  lv_area_t a,b;lv_obj_get_coords(newapi_lists[0],&a);lv_obj_get_coords(newapi_lists[1],&b);assert(a.x2<b.x1&&a.y2<480&&b.y2<480);assert(!lv_obj_has_flag(newapi_names[0][0],LV_OBJ_FLAG_CLICKABLE));
  newapi_screenshot("build/native/newapi-dashboard.ppm");dock_theme_appearance(0x2563eb,1);newapi_screenshot("build/native/newapi-dashboard-light.ppm");dock_theme_appearance(0x0f766e,0);
  lv_obj_scroll_to_y(newapi_lists[0],72,LV_ANIM_OFF);lv_refr_now(display);int y=lv_obj_get_scroll_y(newapi_lists[0]);assert(y>0);newapi_fixture();lv_refr_now(display);assert(lv_obj_get_scroll_y(newapi_lists[0])==y);assert(lv_obj_get_scroll_y(newapi_lists[1])==0);
  dock_newapi_rows(0,2);assert(newapi_counts[0]==2&&!newapi_rows[0][2]);dock_newapi_rows(0,0);assert(!lv_obj_has_flag(newapi_empty[0],LV_OBJ_FLAG_HIDDEN));dock_newapi_rows(0,1000);assert(newapi_counts[0]==100);
  lv_obj_send_event(newapi_refresh,LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"refresh"));
  lv_obj_send_event(newapi_reset,LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"reset.begin"));
  dock_newapi_render("等待确认",0,10,"10 秒后执行\n每个用户余额设为 $200.00");assert(!lv_obj_has_flag(newapi_overlay,LV_OBJ_FLAG_HIDDEN));assert(lv_obj_has_state(newapi_reset,LV_STATE_DISABLED));newapi_screenshot("build/native/newapi-countdown.ppm");
  lv_obj_send_event(newapi_cancel,LV_EVENT_CLICKED,NULL);assert(dock_poll(&event)==1&&!strcmp(event.action,"reset.cancel"));dock_newapi_render("已取消",0,0,"");assert(lv_obj_has_flag(newapi_overlay,LV_OBJ_FLAG_HIDDEN));
  dock_newapi_render("正在重置",1,0,"");assert(lv_obj_has_state(newapi_reset,LV_STATE_DISABLED)&&lv_obj_has_state(newapi_refresh,LV_STATE_DISABLED));
  if(pass==0){newapi_fixture();double times[60];for(int i=0;i<60;i++){struct timespec a,b;clock_gettime(CLOCK_MONOTONIC,&a);lv_obj_invalidate(newapi_screen);lv_refr_now(display);clock_gettime(CLOCK_MONOTONIC,&b);times[i]=(b.tv_sec-a.tv_sec)*1000.0+(b.tv_nsec-a.tv_nsec)/1000000.0;}qsort(times,60,sizeof(double),newapi_compare);printf("NewAPI single-page warm redraw: n=60 p50_ms=%.3f p95_ms=%.3f\n",times[30],times[57]);}
  dock_show_catalog();dock_newapi_unmount();assert(!newapi_screen&&!newapi_lists[0]);dock_newapi_unmount();
 }
 dock_close();puts("NewAPI single page, metric cards, ranking bars, scrolling, themes and reset passed");return 0;
}
