#ifndef DOCK_PAGE_NEWAPI_H
#define DOCK_PAGE_NEWAPI_H
int dock_newapi_mount(void);
void dock_newapi_unmount(void);
void dock_newapi_render(const char *status,int busy,int countdown,const char *reset);
void dock_newapi_metric(int index,const char *usd,const char *calls,const char *rate);
void dock_newapi_balance(int index,const char *name,const char *value,const char *note);
void dock_newapi_rows(int kind,int count);
void dock_newapi_row(int kind,int index,const char *name,const char *value,int amount);
#endif
