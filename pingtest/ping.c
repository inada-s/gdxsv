/* 
   ping.c
   Copyright (C)2020 Edward Li

   Basic tcp ping program for Flycast
*/

#include <kos.h>
#include <kos/net.h>
#include <kos/thread.h>

#include "dc/modem/modem.h"
#include "mintern.h"
#include <ppp/ppp.h>

#include <netdb.h>
#include <sys/socket.h>
#include <sys/time.h>

KOS_INIT_FLAGS(INIT_DEFAULT | INIT_NET);

#define printf dbgio_printf


void *modem_reg_hack(void *param){
    
    while(!modem_is_connected() && !modem_is_connecting() && (modemRead(REGLOC(0x8)) & 0x1) == 0){
        //by pass assert by race condition
    }
    printf("[ping] Connection hack success!\n");
    
    /* the below reg hack is replaced by the above while loop
    int handshaked = 0;

    while( !modem_is_connected() || modem_is_connecting() ) {
    
        if (modemRead(REGLOC(0xF)) & (1 << (4 - 1)) && !handshaked){
            modemConnection(); //MODEM_STATE_CONNECT_WAIT, required for setting DTR
            modemConnectionAnswerCallback();
            handshaked = 1;
        }
        if (modemRead(REGLOC(0xF)) == 0xd0){
            modemConnection(); //MODEM_STATE_CONNECTING, required for setting RTS
            modemCfg.flags = MODEM_CFG_FLAG_CONNECTED;
            modemConnectedUpdate();
            printf("[ping] Connection hack success\n");
        }
        thd_pass();
    }
    */
    return NULL;
}

int main(){

    ppp_init();

    thd_create(0, modem_reg_hack, NULL);
    
    //init modem manually
    modemHardReset();
    modemDataSetupBuffers();
    modemIntInit();
    modemConfigurationReset();
    modemCfg.eventHandler = NULL;
    modemCfg.inited       = 1;
    
    
    ppp_modem_init("123", 1, NULL);
    ppp_set_login("dream", "cast");

    int i = ppp_connect();

    if(i == -1) {
        printf("[ping] Link establishment failed!\n");
        return 0;
    }else
        printf("[ping] Connected!\n");

    int sockfd = 0;
    sockfd = socket(AF_INET , SOCK_STREAM , 0);
    if (sockfd == -1)
        printf("[ping] Fail to create a socket.\n");

    struct sockaddr_in info;
    bzero(&info,sizeof(info));
    info.sin_family = AF_INET;

    /* Using the DNS address as server address */
    char ip[16];
    sprintf(ip, "%d.%d.%d.%d", net_default_dev->dns[0], net_default_dev->dns[1], net_default_dev->dns[2], net_default_dev->dns[3]);
    info.sin_addr.s_addr = inet_addr(ip);
    // info.sin_addr.s_addr = inet_addr("192.168.20.3");
    info.sin_port = htons(8888);
    
    int err = connect(sockfd, (struct sockaddr *) &info, sizeof(info));
    if(err == -1)
        printf("[ping] Connection error\n");

    //Send a message to server
    //char message[1] = {"\n"};
    char message[19] = {"12345678901234567\n"}; 
    char receiveMessage[100] = {};

    for (int i=0; i<1000; i++) {
        printf("[ping] C->S: %s", message);
        struct timeval stop, start;
        send(sockfd, message, sizeof(message), 0);
        gettimeofday(&start, NULL);
        ssize_t size = recv(sockfd, receiveMessage, sizeof(receiveMessage) - 1, 0); //don't send \0
        gettimeofday(&stop, NULL);
        receiveMessage[size] = '\0';
        printf("[ping] S->C: %s", receiveMessage);
        printf("[ping] took %lu ms\n\n", ((stop.tv_sec - start.tv_sec) * 1000000 + stop.tv_usec - start.tv_usec)/1000 );
    }

    printf("[ping] close Socket\n");
    close(sockfd);

    ppp_shutdown();

    return 0;
}
