typedef unsigned int u32;

#define GDXINIT __attribute__((section("gdx.init")))
#define GDXDATA __attribute__((section("gdx.data")))
#define GDXFUNC __attribute__((section("gdx.func")))
#define BUFSIZE 1024

// Injection
u32 init_injection GDXINIT = 0x0c03e000;
int gdx_data_start GDXDATA = 0;
int gdx_initialized GDXDATA = 0;

// top function will be called to initialize.
void GDXFUNC gdx_initialize() {
  if (gdx_initialized) {
    printf("already initialized\n");
    return;
  }

  printf("initialize\n");
  gdx_initialized = 1;
}


struct gdx_queue {
  u32 head;
  u32 tail;
  char buf[BUFSIZE];
};

int tcp_stat GDXDATA = 0;
int ppp_stat GDXDATA = 0;
struct gdx_queue gdx_rxq GDXDATA;
struct gdx_queue gdx_txq GDXDATA;

// TODO args
int GDXFUNC printf(const char* s) {
  return ((int (*)(const char*))0x00117f48)(s);
}

u32 GDXFUNC gdx_queue_size(struct gdx_queue *q) {
  if (q->tail > q->head) return q->tail - q->head;
  return q->tail + BUFSIZE - q->head;
}

void GDXFUNC gdx_queue_push(struct gdx_queue* q, char data) {
  q->tail = (q->tail + 1) % BUFSIZE;
  q->buf[q->tail] = data;
}

char GDXFUNC gdx_queue_pop(struct gdx_queue* q) {
  char ret = q->buf[q->head];
  q->head = (q->head + 1) % BUFSIZE;
  return ret;
}

u32 GDXFUNC gdx_tcp_stat(u32 _) {
  return gdx_queue_size(&gdx_rxq);
}

GDXFUNC static u32 gdx_tcp_recv(u32 _, char* p, u32 len) {
  u32 i;
  if (gdx_queue_size(&gdx_rxq) < len) {
    return -1;
  }
  for (i = 0; i < len; ++i) {
    p[i] = gdx_queue_pop(&gdx_rxq);
  }
  return len;
}

GDXFUNC static u32 gdx_tcp_send(u32 _, char* p, u32 len) {
  u32 i;
  if (len == 0) return 0;
  for (i = 0; i < len; ++i) {
    gdx_queue_push(&gdx_txq, p[i]);
  }
  return len;
}
