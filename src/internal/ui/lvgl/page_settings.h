#ifndef DOCK_PAGE_SETTINGS_H
#define DOCK_PAGE_SETTINGS_H
void dock_settings_volume(int volume,const char *error);
int dock_settings_mount(void);
int dock_settings_back(void);
void dock_settings_screensaver(const char *id);
void dock_settings_unmount(void);
void dock_settings_render(const char *status,const char *wifi,const char *bt,int on,int ready,int busy);
void dock_settings_clear(void);
void dock_settings_network(const char *id,const char *ssid,const char *security,int signal,int current);
void dock_settings_device(const char *address,const char *name,int paired,int connected);
void dock_settings_theme_add(const char *id,const char *title,unsigned color);
void dock_settings_theme(const char *id);
void dock_settings_mode(const char *mode);
#endif
