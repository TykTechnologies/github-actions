#!/usr/bin/env bash
set -eo pipefail


function set_slack_user() {
    declare -A EMAIL_TO_SLACK_ID_MAP
    EMAIL_TO_SLACK_ID_MAP["97896463+andrei-tyk@users.noreply.github.com"]="<@U02U2QM8NKY>"  #Andrei
    EMAIL_TO_SLACK_ID_MAP["ahmet@mangomm.co.uk"]="<@U742EKF7B>"                             #Ahmet
    EMAIL_TO_SLACK_ID_MAP["excieve@gmail.com"]="<@U5VN52FCJ>"                               #Artem
    EMAIL_TO_SLACK_ID_MAP["alephnull@gmail.com"]="<@UKH957GCW>"                             #Alok
    EMAIL_TO_SLACK_ID_MAP["1187055+asutosh@users.noreply.github.com"]="<@U02SC7C53TQ>"      #Asutosh
    EMAIL_TO_SLACK_ID_MAP["62008772+bojantyk@users.noreply.github.com"]="<@UV4AT3QFR>"      #Bojan
    EMAIL_TO_SLACK_ID_MAP["burak.sezer.developer@gmail.co"]="<@U02R5QY4U8Y>"                #Burak Sezer
    EMAIL_TO_SLACK_ID_MAP["buraksekili@gmail.com"]="<@U02P6G92FDJ>"                         #Burak Sekili
    EMAIL_TO_SLACK_ID_MAP["97617859+caroltyk@users.noreply.github.com"]="<@U02TA392526>"    #Carol
    EMAIL_TO_SLACK_ID_MAP["ermirizio@gmail.com"]="<@U02E5NFTLBD>"                           #Esteban
    EMAIL_TO_SLACK_ID_MAP["firas@tyk.io"]="<@U03M1R720PJ>"                                  #Firas
    EMAIL_TO_SLACK_ID_MAP["66144664+MFCaballero@users.noreply.github.com"]="<@U03HH6WGUCC>" #Flor
    EMAIL_TO_SLACK_ID_MAP["furkan_senharputlu@hotmail.com"]="<@UENT4SJG0>"                  #Furkan
    EMAIL_TO_SLACK_ID_MAP["45805296+gothka@users.noreply.github.com"]="<@U01PU70SBEY>"      #Gowtham
    EMAIL_TO_SLACK_ID_MAP["ilijabojanovic@gmail.com"]="<@U3PA24R0Q>"                        #Ilija
    EMAIL_TO_SLACK_ID_MAP["jeffy.mathew100@gmail.com"]="<@U02DYVB0NVC>"                     #Jeffy
    EMAIL_TO_SLACK_ID_MAP["1618778+joshblakeley@users.noreply.github.com"]="<@U6XQR9JFQ>"   #Josh Blakeley
    EMAIL_TO_SLACK_ID_MAP["komaldsukhani@gmail.com"]="<@UGVUM784E>"                         #Komal
    EMAIL_TO_SLACK_ID_MAP["komuw05@gmail.com"]="<@U027AAU1RHD>"                             #Komu
    EMAIL_TO_SLACK_ID_MAP["konrad@tyk.io"]="<@U014S8HCX8C>"                                 #Konrad
    EMAIL_TO_SLACK_ID_MAP["maciej.borzecki@open-rnd.pl"]="<@UMANCE31D>"                     #Maciej
    EMAIL_TO_SLACK_ID_MAP["manasseh@tyk.io"]="<@U05LKAZ21RR>"                               #Manasseh
    EMAIL_TO_SLACK_ID_MAP["matipvp02@gmail.com"]="<@U03PSP335GF>"                           #Matias
    EMAIL_TO_SLACK_ID_MAP["10356476+mitjaziv@users.noreply.github.com"]="<@U03AH1JKY3H>"    #Mitja
    EMAIL_TO_SLACK_ID_MAP["45770178+kolavcic@users.noreply.github.com"]="<@U01K38FN9K2>"    #Mladen
    EMAIL_TO_SLACK_ID_MAP["laurentiu.ghiur@gmail.com"]="<@U3QNEPT6K>"                       #Laurentiu
    EMAIL_TO_SLACK_ID_MAP["leonsbox@gmail.com"]="<@U3P2L4XNE>"                              #Leo
    EMAIL_TO_SLACK_ID_MAP["long@tyk.io "]="<@U0359SK5V2M>"                                  #Long
    EMAIL_TO_SLACK_ID_MAP["odukoyaonline@gmail.com"]="<@U03PUTZUQ4S>"                       #Odukoya
    EMAIL_TO_SLACK_ID_MAP["padiazg@gmail.com"]="<@U01GZT13R2L>"                             #Pato
    EMAIL_TO_SLACK_ID_MAP["patric@tyk.io"]="<@UTSTY8933>"                                   #Patric
    EMAIL_TO_SLACK_ID_MAP["peter.stubbs@outlook.com"]="<@U024HDQ9DBP>"                      #Pete
    EMAIL_TO_SLACK_ID_MAP["104971506+singhpr@users.noreply.github.com"]="<@U03DRTFDMEW>"    #Pranshu 
    EMAIL_TO_SLACK_ID_MAP["74986539+gwotg@users.noreply.github.com"]="<@U01EZ7B4B1D>"       #Rafael  
    EMAIL_TO_SLACK_ID_MAP["sedkyaboushamalah@gmail.com"]="<@UJ3TZB9DE>"                     #Sedky
    EMAIL_TO_SLACK_ID_MAP["legabox@gmail.com"]="<@U02NAMDFLHZ>"                             #Sergei
    EMAIL_TO_SLACK_ID_MAP["sonja+github@tyk.io"]="<@U03JHCG9Z1R>"                           #Sonja
    EMAIL_TO_SLACK_ID_MAP["sredny.buitrago@gmail.com"]="<@UP94FRRPV>"                       #Sredny
    EMAIL_TO_SLACK_ID_MAP["tombuchaillot89@gmail.com"]="<@UPAFZ3NGK>"                       #Tomas
    EMAIL_TO_SLACK_ID_MAP["black@scene-si.org"]="<@U0354KM0M28>"                            #Tit
    EMAIL_TO_SLACK_ID_MAP["40038442+philcz16@users.noreply.github.com"]="<@U02TZRSMPJ4>"    #Terpase
    EMAIL_TO_SLACK_ID_MAP["umituunal@gmail.com"]="<@U026Y78HL2Y>"                           #Umit
    EMAIL_TO_SLACK_ID_MAP["ifrimvlad@gmail.com"]="<@ULM9K98N7>"                             #Vlad
    EMAIL_TO_SLACK_ID_MAP["wojciech@tyk.io"]="<@U01R0RJ8CJ2>"                               #Wojciech
    EMAIL_TO_SLACK_ID_MAP["3155222+letzya@users.noreply.github.com"]="<@U6SJ24M53>"         #Yaara  
    EMAIL_TO_SLACK_ID_MAP["zaid@tyk.io"]="<@U01EW8K8Q1M>"                                   #Zaid
    

    CORRESPONDING_SLACK_MEMBER_ID="${EMAIL_TO_SLACK_ID_MAP[${EMAIL}]}"

    if [ -n "$CORRESPONDING_SLACK_MEMBER_ID" ]; then
        echo "slack_user_name=$CORRESPONDING_SLACK_MEMBER_ID" >> $GITHUB_OUTPUTS
    else
        echo "No corresponding Slack member ID found for the user email."
    fi
}


if [[ -z ${EMAIL} ]];then
    echo "Github email is not defined. Please set EMAIL environment variable"
    exit 1
fi

set_slack_user