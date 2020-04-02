
/Users/shingo/Dropbox/MMBBemu/gdxsv-patch/patch/gdxpatch.o:     file format elf32-littlemips

Disassembly of section gdx.init:

0015c4e8 <init_injection>:
  15c4e8:	0c03e000 	jal	f8000 <gdx_initialize>
Disassembly of section gdx.data:

000f0000 <gdx_data_start>:
   f0000:	00000000 	nop

000f0004 <gdx_initialized>:
   f0004:	00000000 	nop

000f0008 <tcp_stat>:
   f0008:	00000000 	nop

000f000c <ppp_stat>:
   f000c:	00000000 	nop

000f0010 <gdx_rxq>:
	...

000f0418 <gdx_txq>:
	...
Disassembly of section gdx.func:

000f8000 <gdx_initialize>:
   f8000:	27bdffe0 	addiu	sp,sp,-32
   f8004:	ffbf0010 	sd	ra,16(sp)
   f8008:	ffbe0000 	sd	s8,0(sp)
   f800c:	03a0f02d 	move	s8,sp
   f8010:	3c02000f 	lui	v0,0xf
   f8014:	8c420004 	lw	v0,4(v0)
   f8018:	10400006 	beqz	v0,f8034 <gdx_initialize+0x34>
   f801c:	00000000 	nop
   f8020:	3c040010 	lui	a0,0x10
   f8024:	0c03e018 	jal	f8060 <printf>
   f8028:	24848358 	addiu	a0,a0,-31912
   f802c:	10000007 	b	f804c <gdx_initialize+0x4c>
   f8030:	00000000 	nop
   f8034:	3c040010 	lui	a0,0x10
   f8038:	0c03e018 	jal	f8060 <printf>
   f803c:	24848370 	addiu	a0,a0,-31888
   f8040:	24020001 	li	v0,1
   f8044:	3c01000f 	lui	at,0xf
   f8048:	ac220004 	sw	v0,4(at)
   f804c:	03c0e82d 	move	sp,s8
   f8050:	dfbf0010 	ld	ra,16(sp)
   f8054:	dfbe0000 	ld	s8,0(sp)
   f8058:	03e00008 	jr	ra
   f805c:	27bd0020 	addiu	sp,sp,32

000f8060 <printf>:
   f8060:	27bdffd0 	addiu	sp,sp,-48
   f8064:	ffbf0020 	sd	ra,32(sp)
   f8068:	ffbe0010 	sd	s8,16(sp)
   f806c:	03a0f02d 	move	s8,sp
   f8070:	afc40000 	sw	a0,0(s8)
   f8074:	8fc40000 	lw	a0,0(s8)
   f8078:	3c010011 	lui	at,0x11
   f807c:	34217f48 	ori	at,at,0x7f48
   f8080:	0020f809 	jalr	at
   f8084:	00000000 	nop
   f8088:	03c0e82d 	move	sp,s8
   f808c:	dfbf0020 	ld	ra,32(sp)
   f8090:	dfbe0010 	ld	s8,16(sp)
   f8094:	03e00008 	jr	ra
   f8098:	27bd0030 	addiu	sp,sp,48

000f809c <gdx_queue_size>:
   f809c:	27bdffe0 	addiu	sp,sp,-32
   f80a0:	ffbe0010 	sd	s8,16(sp)
   f80a4:	03a0f02d 	move	s8,sp
   f80a8:	afc40000 	sw	a0,0(s8)
   f80ac:	8fc20000 	lw	v0,0(s8)
   f80b0:	8fc30000 	lw	v1,0(s8)
   f80b4:	8c440004 	lw	a0,4(v0)
   f80b8:	8c620000 	lw	v0,0(v1)
   f80bc:	0044102b 	sltu	v0,v0,a0
   f80c0:	10400008 	beqz	v0,f80e4 <gdx_queue_size+0x48>
   f80c4:	00000000 	nop
   f80c8:	8fc20000 	lw	v0,0(s8)
   f80cc:	8fc30000 	lw	v1,0(s8)
   f80d0:	8c440004 	lw	a0,4(v0)
   f80d4:	8c620000 	lw	v0,0(v1)
   f80d8:	00821023 	subu	v0,a0,v0
   f80dc:	10000008 	b	f8100 <gdx_queue_size+0x64>
   f80e0:	afc20004 	sw	v0,4(s8)
   f80e4:	8fc20000 	lw	v0,0(s8)
   f80e8:	8fc30000 	lw	v1,0(s8)
   f80ec:	8c440004 	lw	a0,4(v0)
   f80f0:	8c620000 	lw	v0,0(v1)
   f80f4:	00821023 	subu	v0,a0,v0
   f80f8:	24420400 	addiu	v0,v0,1024
   f80fc:	afc20004 	sw	v0,4(s8)
   f8100:	8fc20004 	lw	v0,4(s8)
   f8104:	03c0e82d 	move	sp,s8
   f8108:	dfbe0010 	ld	s8,16(sp)
   f810c:	03e00008 	jr	ra
   f8110:	27bd0020 	addiu	sp,sp,32

000f8114 <gdx_queue_push>:
   f8114:	27bdffe0 	addiu	sp,sp,-32
   f8118:	ffbe0010 	sd	s8,16(sp)
   f811c:	03a0f02d 	move	s8,sp
   f8120:	afc40000 	sw	a0,0(s8)
   f8124:	00a0102d 	move	v0,a1
   f8128:	a3c20004 	sb	v0,4(s8)
   f812c:	8fc30000 	lw	v1,0(s8)
   f8130:	8fc20000 	lw	v0,0(s8)
   f8134:	8c420004 	lw	v0,4(v0)
   f8138:	24420001 	addiu	v0,v0,1
   f813c:	304203ff 	andi	v0,v0,0x3ff
   f8140:	ac620004 	sw	v0,4(v1)
   f8144:	8fc30000 	lw	v1,0(s8)
   f8148:	8fc20000 	lw	v0,0(s8)
   f814c:	8c420004 	lw	v0,4(v0)
   f8150:	00621821 	addu	v1,v1,v0
   f8154:	93c20004 	lbu	v0,4(s8)
   f8158:	a0620008 	sb	v0,8(v1)
   f815c:	03c0e82d 	move	sp,s8
   f8160:	dfbe0010 	ld	s8,16(sp)
   f8164:	03e00008 	jr	ra
   f8168:	27bd0020 	addiu	sp,sp,32

000f816c <gdx_queue_pop>:
   f816c:	27bdffe0 	addiu	sp,sp,-32
   f8170:	ffbe0010 	sd	s8,16(sp)
   f8174:	03a0f02d 	move	s8,sp
   f8178:	afc40000 	sw	a0,0(s8)
   f817c:	8fc30000 	lw	v1,0(s8)
   f8180:	8fc20000 	lw	v0,0(s8)
   f8184:	8c420000 	lw	v0,0(v0)
   f8188:	00621021 	addu	v0,v1,v0
   f818c:	90420008 	lbu	v0,8(v0)
   f8190:	a3c20004 	sb	v0,4(s8)
   f8194:	8fc30000 	lw	v1,0(s8)
   f8198:	8fc20000 	lw	v0,0(s8)
   f819c:	8c420000 	lw	v0,0(v0)
   f81a0:	24420001 	addiu	v0,v0,1
   f81a4:	304203ff 	andi	v0,v0,0x3ff
   f81a8:	ac620000 	sw	v0,0(v1)
   f81ac:	83c20004 	lb	v0,4(s8)
   f81b0:	03c0e82d 	move	sp,s8
   f81b4:	dfbe0010 	ld	s8,16(sp)
   f81b8:	03e00008 	jr	ra
   f81bc:	27bd0020 	addiu	sp,sp,32

000f81c0 <gdx_tcp_stat>:
   f81c0:	27bdffd0 	addiu	sp,sp,-48
   f81c4:	ffbf0020 	sd	ra,32(sp)
   f81c8:	ffbe0010 	sd	s8,16(sp)
   f81cc:	03a0f02d 	move	s8,sp
   f81d0:	afc40000 	sw	a0,0(s8)
   f81d4:	3c04000f 	lui	a0,0xf
   f81d8:	24840010 	addiu	a0,a0,16
   f81dc:	0c03e027 	jal	f809c <gdx_queue_size>
   f81e0:	00000000 	nop
   f81e4:	03c0e82d 	move	sp,s8
   f81e8:	dfbf0020 	ld	ra,32(sp)
   f81ec:	dfbe0010 	ld	s8,16(sp)
   f81f0:	03e00008 	jr	ra
   f81f4:	27bd0030 	addiu	sp,sp,48

000f81f8 <gdx_tcp_recv>:
   f81f8:	27bdffc0 	addiu	sp,sp,-64
   f81fc:	ffbf0030 	sd	ra,48(sp)
   f8200:	ffbe0020 	sd	s8,32(sp)
   f8204:	03a0f02d 	move	s8,sp
   f8208:	afc40000 	sw	a0,0(s8)
   f820c:	afc50004 	sw	a1,4(s8)
   f8210:	afc60008 	sw	a2,8(s8)
   f8214:	3c04000f 	lui	a0,0xf
   f8218:	24840010 	addiu	a0,a0,16
   f821c:	0c03e027 	jal	f809c <gdx_queue_size>
   f8220:	00000000 	nop
   f8224:	8fc30008 	lw	v1,8(s8)
   f8228:	0043102b 	sltu	v0,v0,v1
   f822c:	10400004 	beqz	v0,f8240 <gdx_tcp_recv+0x48>
   f8230:	00000000 	nop
   f8234:	2402ffff 	li	v0,-1
   f8238:	10000018 	b	f829c <gdx_tcp_recv+0xa4>
   f823c:	afc20010 	sw	v0,16(s8)
   f8240:	afc0000c 	sw	zero,12(s8)
   f8244:	8fc2000c 	lw	v0,12(s8)
   f8248:	8fc30008 	lw	v1,8(s8)
   f824c:	0043102b 	sltu	v0,v0,v1
   f8250:	14400003 	bnez	v0,f8260 <gdx_tcp_recv+0x68>
   f8254:	00000000 	nop
   f8258:	1000000e 	b	f8294 <gdx_tcp_recv+0x9c>
   f825c:	00000000 	nop
   f8260:	3c04000f 	lui	a0,0xf
   f8264:	24840010 	addiu	a0,a0,16
   f8268:	0c03e05b 	jal	f816c <gdx_queue_pop>
   f826c:	00000000 	nop
   f8270:	0040202d 	move	a0,v0
   f8274:	8fc30004 	lw	v1,4(s8)
   f8278:	8fc2000c 	lw	v0,12(s8)
   f827c:	00621021 	addu	v0,v1,v0
   f8280:	a0440000 	sb	a0,0(v0)
   f8284:	8fc2000c 	lw	v0,12(s8)
   f8288:	24420001 	addiu	v0,v0,1
   f828c:	1000ffed 	b	f8244 <gdx_tcp_recv+0x4c>
   f8290:	afc2000c 	sw	v0,12(s8)
   f8294:	8fc20008 	lw	v0,8(s8)
   f8298:	afc20010 	sw	v0,16(s8)
   f829c:	8fc20010 	lw	v0,16(s8)
   f82a0:	03c0e82d 	move	sp,s8
   f82a4:	dfbf0030 	ld	ra,48(sp)
   f82a8:	dfbe0020 	ld	s8,32(sp)
   f82ac:	03e00008 	jr	ra
   f82b0:	27bd0040 	addiu	sp,sp,64

000f82b4 <gdx_tcp_send>:
   f82b4:	27bdffc0 	addiu	sp,sp,-64
   f82b8:	ffbf0030 	sd	ra,48(sp)
   f82bc:	ffbe0020 	sd	s8,32(sp)
   f82c0:	03a0f02d 	move	s8,sp
   f82c4:	afc40000 	sw	a0,0(s8)
   f82c8:	afc50004 	sw	a1,4(s8)
   f82cc:	afc60008 	sw	a2,8(s8)
   f82d0:	8fc20008 	lw	v0,8(s8)
   f82d4:	14400003 	bnez	v0,f82e4 <gdx_tcp_send+0x30>
   f82d8:	00000000 	nop
   f82dc:	10000017 	b	f833c <gdx_tcp_send+0x88>
   f82e0:	afc00010 	sw	zero,16(s8)
   f82e4:	afc0000c 	sw	zero,12(s8)
   f82e8:	8fc2000c 	lw	v0,12(s8)
   f82ec:	8fc30008 	lw	v1,8(s8)
   f82f0:	0043102b 	sltu	v0,v0,v1
   f82f4:	14400003 	bnez	v0,f8304 <gdx_tcp_send+0x50>
   f82f8:	00000000 	nop
   f82fc:	1000000d 	b	f8334 <gdx_tcp_send+0x80>
   f8300:	00000000 	nop
   f8304:	8fc30004 	lw	v1,4(s8)
   f8308:	8fc2000c 	lw	v0,12(s8)
   f830c:	00621021 	addu	v0,v1,v0
   f8310:	80420000 	lb	v0,0(v0)
   f8314:	3c04000f 	lui	a0,0xf
   f8318:	24840418 	addiu	a0,a0,1048
   f831c:	0c03e045 	jal	f8114 <gdx_queue_push>
   f8320:	0040282d 	move	a1,v0
   f8324:	8fc2000c 	lw	v0,12(s8)
   f8328:	24420001 	addiu	v0,v0,1
   f832c:	1000ffee 	b	f82e8 <gdx_tcp_send+0x34>
   f8330:	afc2000c 	sw	v0,12(s8)
   f8334:	8fc20008 	lw	v0,8(s8)
   f8338:	afc20010 	sw	v0,16(s8)
   f833c:	8fc20010 	lw	v0,16(s8)
   f8340:	03c0e82d 	move	sp,s8
   f8344:	dfbf0030 	ld	ra,48(sp)
   f8348:	dfbe0020 	ld	s8,32(sp)
   f834c:	03e00008 	jr	ra
   f8350:	27bd0040 	addiu	sp,sp,64
   f8354:	00000000 	nop
   f8358:	65726c61 	daddiu	s2,t3,27745
   f835c:	20796461 	addi	t9,v1,25697
   f8360:	74696e69 	jalx	1a5b9a4 <init_injection+0x18ff4bc>
   f8364:	696c6169 	ldl	t4,24937(t3)
   f8368:	0a64657a 	j	99195e8 <init_injection+0x97bd100>
   f836c:	00000000 	nop
   f8370:	74696e69 	jalx	1a5b9a4 <init_injection+0x18ff4bc>
   f8374:	696c6169 	ldl	t4,24937(t3)
   f8378:	000a657a 	dsrl	t4,t2,0x15
   f837c:	00000000 	nop
