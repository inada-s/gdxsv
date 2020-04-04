#include <stdarg.h>

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;

#define GDXDATA __attribute__((section("gdx.data")))
#define GDXFUNC __attribute__((section("gdx.func")))
#define GDXMAIN __attribute__((section("gdx.main")))
u32 gdx_injection __attribute__((section("gdx.inject"))) = 0x0c03ffc0;

#define BUFSIZE 4096
#define OP_NOP 0
#define OP_JR_RA 0x03e00008

struct gdx_queue {
  char name[4];
  u32 head;
  u32 tail;
  u8 buf[BUFSIZE];
};

int gdx_data_start GDXDATA;
int gdx_debug_print GDXDATA = 1;
int gdx_initialized GDXDATA = 0;
struct gdx_queue gdx_rxq GDXDATA = {"rxq"};
struct gdx_queue gdx_txq GDXDATA = {"txq"};


void GDXFUNC gdx_debug(const char *format, ...) {
  if (gdx_debug_print) {
    va_list arg;
    int done;
    va_start(arg, format);
    done = ((int (*)(int *, const char *, va_list arg))0x001192b8)(0x003a73c4, format, arg);
    va_end(arg);
    return done;
  }
}

void GDXFUNC gdx_log(const char *format, ...) {
  va_list arg;
  int done;
  va_start(arg, format);
  done = ((int (*)(int *, const char *, va_list arg))0x001192b8)(0x003a73c4, format, arg);
  va_end(arg);
  return done;
}

u32 GDXFUNC read32(u32 addr) {
  u32* p = addr;
  return *p;
}

u16 GDXFUNC read16(u32 addr) {
  u16* p = addr;
  return *p;
}

u8 GDXFUNC read8(u32 addr) {
  u8* p = addr;
  return *p;
}

void GDXFUNC write32(u32 addr, u32 value) {
  u32* p = addr;
  *p = value;
}

void GDXFUNC write16(u32 addr, u16 value) {
  u16* p = addr;
  *p = value;
}

void GDXFUNC write8(u32 addr, u8 value) {
  u8* p = addr;
  *p = value;
}

u32 GDXFUNC OP_JAL(u32 addr) {
  return 0x0c000000 + addr / 4;
}

u32 GDXFUNC gdx_queue_init(struct gdx_queue *q) {
  q->head = 0;
  q->tail = 0;
}

u32 GDXFUNC gdx_queue_size(struct gdx_queue *q) {
  return (q->tail + BUFSIZE - q->head) % BUFSIZE;
}

u32 GDXFUNC gdx_queue_avail(struct gdx_queue *q) {
  return BUFSIZE - gdx_queue_size(q) - 1;
}

void GDXFUNC gdx_queue_push(struct gdx_queue* q, u8 data) {
  q->buf[q->tail] = data;
  q->tail = (q->tail + 1) % BUFSIZE;
}

u8 GDXFUNC gdx_queue_pop(struct gdx_queue* q) {
  u8 ret = q->buf[q->head];
  q->head = (q->head + 1) % BUFSIZE;
  return ret;
}

u32 GDXFUNC gdx_TcpGetStatus(u32 sock, u32 dst) {
  u16 retvalue = -1;
  u16 readable_size = 0;
  const int n = gdx_queue_size(&gdx_rxq);
  if (0 < n) {
      retvalue = 0;
      readable_size = n <= 0x7fff ? n : 0x7fff;
  }
  write32(dst, 0);
  write32(dst + 4, readable_size);
  return retvalue;
}

u32 GDXFUNC gdx_Ave_TcpSend(u32 sock, u32 ptr, u32 len) {
  int i;
  gdx_debug("gdx_Ave_TcpSend sock:%d ptr:%08x size:%d\n", sock, ptr, len);
  if (len == 0) {
    return 0;
  }

  if (gdx_queue_avail(&gdx_txq) < len) {
    return 0;
  }

  gdx_debug("send:");
  for (i = 0; i < len; ++i) {
    gdx_debug("%02x ", read8(ptr + i));
  }
  gdx_debug("\n");

  for (i = 0; i < len; ++i) {
    gdx_queue_push(&gdx_txq, read8(ptr + i));
  }

  return len;
}

u32 GDXFUNC gdx_Ave_TcpRecv(u32 sock, u32 ptr, u32 len) {
  int i;
  gdx_debug("gdx_Ave_TcpRecv sock:%d ptr:%08x size:%d\n", sock, ptr, len);
  if (gdx_queue_size(&gdx_rxq) < len) {
    return -1;
  }

  for (i = 0; i < len; ++i) {
    write8(ptr + i, gdx_queue_pop(&gdx_rxq));
  }

  gdx_debug("recv:");
  for (i = 0; i < len; ++i) {
    gdx_debug("%02x ", read8(ptr + i));
  }
  gdx_debug("\n");

  return len;
}

void GDXFUNC gdx_McsReceive(u32 ptr, u32 len) {
  int i;
  gdx_debug("gdx_McsReceive ptr:%08x size:%d\n", ptr, len);

  if (len == 0) {
    return 0;
  }

  if (gdx_queue_size(&gdx_rxq) < len) {
    len = gdx_queue_size(&gdx_rxq);
  }

  if (len == 0) {
    return -1;
  }

  for (i = 0; i < len; ++i) {
    write8(ptr + i, gdx_queue_pop(&gdx_rxq));
  }

  gdx_debug("recv:");
  for (i = 0; i < len; ++i) {
    gdx_debug("%02x ", read8(ptr + i));
  }
  gdx_debug("\n");

  return len;
}

void GDXFUNC gdx_LobbyToMcsInitSocket() {
  gdx_queue_init(&gdx_rxq);
  gdx_queue_init(&gdx_txq);
}

void GDXFUNC patch_skip_modem()
{
    // replace modem_recognition with network_battle.
    write32(0x003c4f58, 0x0015f110);

    // skip ppp dialing step.
    write32(0x0035a660, 0x24030002);
}

void GDXFUNC patch_tcp() {
  write32(0x00381fb4, OP_JAL(gdx_Ave_TcpSend));
  write32(0x00381f7c, OP_JAL(gdx_Ave_TcpRecv));
  write32(0x0037fd2c, OP_JAL(gdx_McsReceive));
  write32(0x00357e34, OP_JAL(gdx_TcpGetStatus));
  write32(0x0035a174, OP_JAL(gdx_LobbyToMcsInitSocket));
}

void GDXMAIN gdx_main() {
  gdx_debug("gdx_main\n");

  if (gdx_initialized) {
    gdx_debug("already initialized\n");
    return;
  }

  patch_skip_modem();
  patch_tcp();
  gdx_queue_init(&gdx_rxq);
  gdx_queue_init(&gdx_txq);

  gdx_initialized = 1;
}
