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

enum {
  RPC_TCP_OPEN = 1,
  RPC_TCP_CLOSE = 2,
};

struct gdx_rpc_t {
  char tag[4];
  u32 request;
  u32 response;

  u32 param1;
  u32 param2;
  u32 param3;
  u32 param4;
  u8 name1[128];
  u8 name2[128];
};

struct gdx_queue {
  char tag[4];
  u32 head;
  u32 tail;
  u8 buf[BUFSIZE];
};

int gdx_data_start GDXDATA = 1;
int gdx_debug_print GDXDATA = 1;
int gdx_initialized GDXDATA = 0;

struct gdx_rpc_t gdx_rpc GDXDATA = {"rpc"};
struct gdx_queue gdx_rxq GDXDATA = {"rxq"};
struct gdx_queue gdx_txq GDXDATA = {"txq"};

int GDXFUNC gdx_debug(const char *format, ...) {
  if (gdx_debug_print) {
    va_list arg;
    int done;
    va_start(arg, format);
    done = ((int (*)(int *, const char *, va_list arg))0x001192b8)(0x003a73c4, format, arg);
    va_end(arg);
    return done;
  }
}

int GDXFUNC gdx_info(const char *format, ...) {
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
  u32 retvalue = -1;
  u32 readable_size = 0;
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

u32 GDXFUNC gdx_McsReceive(u32 ptr, u32 len) {
  int i;
  gdx_debug("gdx_McsReceive ptr:%08x size:%d\n", ptr, len);

  if (len == 0) {
    return 0;
  }

  gdx_debug("gdx_queue_size size:%d\n", gdx_queue_size(&gdx_rxq));
  if (gdx_queue_size(&gdx_rxq) < len) {
    len = gdx_queue_size(&gdx_rxq);
  }

  if (len == 0) {
    return 0;
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

u32 GDXFUNC gdx_Ave_TcpOpen(u32 ip, u32 port) {
  gdx_info("gdx_Ave_TcpOpen\n");
  // u32 ret = ((u32 (*)())0x00350630)(); // TCPInitialize()
  // gdx_info("ret=%d\n", ret);

  // endian fix
  port = port >> 8 | (port & 0xFF) << 8;

  gdx_queue_init(&gdx_rxq);
  gdx_queue_init(&gdx_txq);
  gdx_rpc.request = RPC_TCP_OPEN;
  gdx_rpc.param1 = ip == 0x0077; // to lobby
  gdx_rpc.param2 = ip;
  gdx_rpc.param3 = port;
  return 7; // dummy socket id
}

u32 GDXFUNC gdx_Ave_TcpClose(u32 sock) {
  gdx_info("gdx_Ave_TcpClose\n");
  gdx_queue_init(&gdx_rxq);
  gdx_queue_init(&gdx_txq);
  gdx_rpc.request = RPC_TCP_CLOSE;
  gdx_rpc.param1 = sock;
  return 0;
}

void GDXFUNC gdx_LobbyToMcsInitSocket() {
  gdx_queue_init(&gdx_rxq);
  gdx_queue_init(&gdx_txq);
}

u32 GDXFUNC gdx_gethostbyname_ps2_0(u32 hostname) {
  gdx_info("gdx_gethostbyname_ps2_0\n");
  // hostname : "ca1202.mmcp6"
  return 7; // dummy ticket id
}

u32 GDXFUNC gdx_gethostbyname_ps2_1(u32 ticket_id) {
  gdx_info("gdx_gethostbyname_ps2_1\n");
  return 0x0077; // dummy lobby ip addr
}

u32 GDXFUNC gdx_gethostbyname_ps2_release(u32 ticket_id) {
  gdx_info("gdx_gethostbyname_ps2_release\n");
  return 0; // ok
}

void GDXFUNC write_patch() {
  // replace modem_recognition with network_battle.
  write32(0x003c4f58, 0x0015f110);

  // skip ppp dialing step.
  write32(0x0035a660, 0x24030002);

  // replace network functions.
  write32(0x00381da4, OP_JAL(gdx_Ave_TcpOpen));
  write32(0x00382024, OP_JAL(gdx_Ave_TcpClose));
  write32(0x00381fb4, OP_JAL(gdx_Ave_TcpSend));
  write32(0x00381f7c, OP_JAL(gdx_Ave_TcpRecv));
  write32(0x0037fd2c, OP_JAL(gdx_McsReceive));
  write32(0x00357e34, OP_JAL(gdx_TcpGetStatus));
  write32(0x0035a174, OP_JAL(gdx_LobbyToMcsInitSocket));
  write32(0x00359e04, OP_JAL(gdx_gethostbyname_ps2_0));
  write32(0x00359e78, OP_JAL(gdx_gethostbyname_ps2_1));
  write32(0x00359ea4, OP_JAL(gdx_gethostbyname_ps2_release));
  write32(0x00359ec4, OP_JAL(gdx_gethostbyname_ps2_release));
}

void GDXMAIN gdx_main() {
  gdx_debug("gdx_main\n");

  if (gdx_initialized) {
    gdx_debug("already initialized\n");
    return;
  }

  write_patch();
  gdx_queue_init(&gdx_rxq);
  gdx_queue_init(&gdx_txq);

  gdx_initialized = 1;
}
