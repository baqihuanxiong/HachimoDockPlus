//go:build lvgl && cgo
#include "ui_native.h"
#include "bridge.h"
static lv_style_t accent_button,accent_surface,accent_text;
static lv_display_t *theme_display;
static unsigned theme_primary=0x0f766e;
static int theme_light;
typedef struct{unsigned dark,light;lv_style_t style;} theme_role;
static theme_role backgrounds[]={
 {0x0c1220,0xf3f5f9},{0x121d2c,0xe5eaf2},{0x172333,0xffffff},
 {0x182438,0xffffff},{0x1e293b,0xe2e8f0},{0x334155,0xd8e0ed},{0x203147,0xedf1f8}
};
static theme_role foregrounds[]={
 {0xf1f5f9,0x0f172a},{0xf8fafc,0x0f172a},{0xe2e8f0,0x1e293b},
 {0xcbd5e1,0x334155},{0x94a3b8,0x52627a},{0xfbbf24,0xa16207}
};
#define ROLE_COUNT(a) (sizeof(a)/sizeof((a)[0]))
static unsigned blend(unsigned rgb,unsigned base,unsigned weight){
 unsigned r=(((rgb>>16)&255)*weight+((base>>16)&255)*(255-weight))/255;
 unsigned g=(((rgb>>8)&255)*weight+((base>>8)&255)*(255-weight))/255;
 unsigned b=((rgb&255)*weight+(base&255)*(255-weight))/255;return (r<<16)|(g<<8)|b;
}
unsigned dock_theme_resolve(unsigned rgb){
 if(!theme_light)return rgb;
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)if(backgrounds[i].dark==rgb)return backgrounds[i].light;
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)if(foregrounds[i].dark==rgb)return foregrounds[i].light;
 switch(rgb){case 0x475569:return 0xcbd5e1;case 0x4ade80:return 0x16a34a;case 0xfb923c:return 0xd97706;case 0xf87171:return 0xdc2626;}
 return rgb;
}
void dock_theme_appearance(unsigned rgb,int light){
 theme_primary=rgb&0xffffff;theme_light=!!light;
 lv_style_set_bg_color(&accent_button,dock_color(theme_primary));
 lv_style_set_bg_color(&accent_surface,dock_color(blend(theme_primary,theme_light?0xffffff:0x121d2c,theme_light?30:85)));
 lv_style_set_text_color(&accent_text,dock_color(theme_light?theme_primary:blend(theme_primary,0xffffff,110)));
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)lv_style_set_bg_color(&backgrounds[i].style,dock_color(theme_light?backgrounds[i].light:backgrounds[i].dark));
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)lv_style_set_text_color(&foregrounds[i].style,dock_color(theme_light?foregrounds[i].light:foregrounds[i].dark));
 if(theme_display){
  lv_theme_t *theme=lv_theme_default_init(theme_display,dock_color(theme_primary),dock_color(0x6366f1),!theme_light,dock_font_body);
  lv_display_set_theme(theme_display,theme);
 }
 lv_obj_report_style_change(&accent_button);lv_obj_report_style_change(&accent_surface);lv_obj_report_style_change(&accent_text);
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)lv_obj_report_style_change(&backgrounds[i].style);
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)lv_obj_report_style_change(&foregrounds[i].style);
 if(theme_display)lv_obj_invalidate(lv_display_get_screen_active(theme_display));
}
void dock_theme_set(unsigned rgb){dock_theme_appearance(rgb,theme_light);}
void dock_theme_init(lv_display_t *display){
 lv_style_init(&accent_button);lv_style_init(&accent_surface);lv_style_init(&accent_text);
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)lv_style_init(&backgrounds[i].style);
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)lv_style_init(&foregrounds[i].style);
 theme_display=display;dock_theme_appearance(0x0f766e,0);
}
void dock_theme_close(void){
 theme_display=NULL;lv_style_reset(&accent_button);lv_style_reset(&accent_surface);lv_style_reset(&accent_text);
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)lv_style_reset(&backgrounds[i].style);
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)lv_style_reset(&foregrounds[i].style);
}
void dock_theme_background(lv_obj_t *obj,unsigned rgb,lv_style_selector_t selector){
 lv_obj_remove_local_style_prop(obj,LV_STYLE_BG_COLOR,selector);
 lv_obj_remove_style(obj,&accent_button,selector);lv_obj_remove_style(obj,&accent_surface,selector);
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)lv_obj_remove_style(obj,&backgrounds[i].style,selector);
 if(rgb==0x0f766e){lv_obj_add_style(obj,&accent_button,selector);return;}
 if(rgb==0x134e4a){lv_obj_add_style(obj,&accent_surface,selector);return;}
 for(unsigned i=0;i<ROLE_COUNT(backgrounds);i++)if(rgb==backgrounds[i].dark){lv_obj_add_style(obj,&backgrounds[i].style,selector);return;}
 lv_obj_set_style_bg_color(obj,dock_color(rgb),selector);
}
void dock_theme_foreground(lv_obj_t *obj,unsigned rgb,lv_style_selector_t selector){
 lv_obj_remove_local_style_prop(obj,LV_STYLE_TEXT_COLOR,selector);lv_obj_remove_style(obj,&accent_text,selector);
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)lv_obj_remove_style(obj,&foregrounds[i].style,selector);
 if(rgb==0x5eead4){lv_obj_add_style(obj,&accent_text,selector);return;}
 for(unsigned i=0;i<ROLE_COUNT(foregrounds);i++)if(rgb==foregrounds[i].dark){lv_obj_add_style(obj,&foregrounds[i].style,selector);return;}
 lv_obj_set_style_text_color(obj,dock_color(rgb),selector);
}
void dock_theme_button(lv_obj_t *obj,int active){
 dock_theme_background(obj,active?0x0f766e:0x334155,0);
 if(lv_obj_get_child_count(obj)){lv_obj_t *label=lv_obj_get_child(obj,0);if(lv_obj_check_type(label,&lv_label_class))dock_theme_foreground(label,active?0xffffff:0xe2e8f0,0);}
}
void dock_theme_surface(lv_obj_t *obj){dock_theme_background(obj,0x134e4a,0);}
void dock_theme_text(lv_obj_t *obj){dock_theme_foreground(obj,0x5eead4,0);}
