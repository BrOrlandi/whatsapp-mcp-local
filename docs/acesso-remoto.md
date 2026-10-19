# Acesso remoto: o app como controle de um servidor

O WhatsApp MCP pode rodar num servidor que fica ligado o tempo todo, e o app
de um computador vira o controle dele: o painel, o pareamento do WhatsApp, as
configurações e o MCP das ferramentas de IA desse computador passam a ser os
do servidor. Este documento diz como a conexão funciona, o que ela garante e o
que não garante. Para instalar, veja [Num servidor sem tela](servidor.md).

## Em uma frase

O app e o servidor conversam só por HTTPS, e dentro do HTTPS por uma sessão
do [Noise Protocol Framework](https://noiseprotocol.org/noise.html) entre os
dois: cada ponta prova que tem a sua chave, e o proxy que termina o HTTPS no
caminho só repassa bytes que não consegue ler nem alterar.

```
 Mac                                   servidor
┌────────────────────┐   HTTPS   ┌─────────┐  rede   ┌──────────────────────┐
│ app (modo remoto)  │──────────►│  proxy  │────────►│ whatsapp-mcp serve   │
│  127.0.0.1:47821   │  Noise IK │ (Traefik│ privada │  /remote/v1/session  │
│  ↑ Claude, Codex,  │◄──────────│  etc.)  │◄────────│  painel, API, MCP    │
│    linha de comando│  (cifrado)└─────────┘         │  só em 127.0.0.1     │
└────────────────────┘                               └──────────────────────┘
```

## O que acontece

**No servidor**, o daemon escuta em dois lugares:

- em `127.0.0.1:47821`, como sempre: o painel, a API e o MCP, só para
  programas do próprio servidor;
- com `WHATSAPP_MCP_REMOTE_LISTEN`, num endereço da rede privada do proxy:
  só três rotas, `/remote/v1/pair`, `/remote/v1/session` e
  `/remote/v1/health`. O painel, a API e o MCP nunca respondem ali.

**No Mac**, o app no modo remoto não roda o wacli. Ele mantém uma sessão com
o servidor e serve, no seu próprio `127.0.0.1:47821`, o que o servidor serve:
a janela mostra o painel do servidor, e o Claude Code, o Claude Desktop, o
Codex e a linha de comando continuam apontando para o mesmo endereço de
antes. Cada pedido vai ao servidor dentro da sessão cifrada.

## Pareamento

1. No servidor, alguém com acesso a ele gera um código:
   `whatsapp-mcp remote code` (ou *Configurações › Acesso remoto* no painel
   dele). O código tem um identificador, **32 bytes aleatórios** (`crypto/rand`)
   e o começo da chave pública do servidor (a impressão digital). Ele aparece
   uma vez e **não é gravado**: o segredo fica só na memória do processo, vale
   **10 minutos** e **uma vez**. Ele é gasto quando um aparelho prova que tem o
   segredo; quem souber só o identificador (que não é secreto, e viaja na
   primeira mensagem, fora do endereço que um proxy registra) não o gasta.
2. No app, a pessoa cola o endereço `https://` do servidor e o código. O app
   gera o par de chaves do dispositivo (X25519) e roda o handshake
   `Noise_XXpsk3_25519_ChaChaPoly_SHA256` com o segredo do código como chave
   pré-compartilhada (PSK): as duas pontas trocam as chaves estáticas, e só
   quem tem o segredo completa o handshake.
3. O app confere a chave do servidor com a impressão digital do código e só
   então confirma. O servidor registra o dispositivo depois da confirmação:
   **só a chave pública**, o nome e o escopo, no `state.db`.
4. O app guarda a **chave privada no Keychain do Mac**, só neste aparelho
   (`AfterFirstUnlockThisDeviceOnly`, sem sincronizar com o iCloud), e no
   `config.json` o endereço, a chave pública do servidor e o id do dispositivo.

O segredo do código participa do handshake como PSK, um uso previsto pelo
Noise; nenhum hash dele é guardado nem usado como chave.

## Cada sessão

O app abre um WebSocket em `wss://…/remote/v1/session` e roda
`Noise_IK_25519_ChaChaPoly_SHA256`, com a chave do servidor que guardou no
pareamento:

- **Autenticação das pontas:** o servidor prova que tem a chave privada
  fixada no pareamento (senão o handshake falha no app), e o app prova que tem
  a chave do dispositivo registrada. Uma chave desconhecida, revogada ou
  trocada é recusada.
- **Contra replay do handshake:** a primeira mensagem leva um carimbo de
  tempo (nanossegundos), que precisa ser maior que o último aceito daquele
  dispositivo. Repetir uma primeira mensagem capturada não abre sessão.
- **Chaves de sessão:** o Noise deriva, por HKDF sobre o estado do handshake,
  uma chave para cada sentido; elas são efêmeras (sigilo futuro: vazar a chave
  estática depois não abre sessões passadas). Cada sentido faz `Rekey` a cada
  65.536 mensagens, nas duas pontas ao mesmo tempo.
- **Integridade e ordem:** cada mensagem é cifrada com ChaCha20-Poly1305 e um
  contador como nonce. Uma mensagem alterada, repetida, perdida ou fora de
  ordem não decifra, e a sessão termina na hora.
- **Dentro da sessão**, os pedidos HTTP viajam em quadros (método, caminho,
  cabeçalhos, corpo). Cabeçalhos de conexão e credenciais do lado do app
  (`Authorization`, `Cookie`) não atravessam: no servidor, quem fala é o
  dispositivo autenticado.

## Escopos, revogação e troca de chave

- **Escopo `admin`** (o app): o painel, a API e o MCP do servidor. O que a
  janela do app manda é servido como a janela do painel; o que outro programa
  do computador do app manda pelo `127.0.0.1` dele passa pelas mesmas
  checagens que no servidor (o painel recusa ações sem JSON ou vindas de outro
  site, e o token do servidor vale). **Escopo `tools`**: só o `/mcp`; qualquer
  outra rota responde 403. A administração (parear, revogar, desconectar o
  WhatsApp, webhooks) fica fora do alcance de um dispositivo `tools`.
- **Revogar:** `whatsapp-mcp remote revoke <id>`, ou o botão em
  *Configurações › Acesso remoto*. As sessões abertas do dispositivo caem na
  hora, e ele não reconecta; precisa ser pareado de novo.
- **Trocar a chave do dispositivo:** no app, *Trocar a chave deste app*. O
  app manda a nova chave pública pela sessão atual; o servidor a aceita só
  quando o app abre uma sessão provando que tem a nova, e só então a antiga
  deixa de valer, no servidor e no Keychain. Se o servidor reiniciar no meio,
  o app volta à chave atual, que continua valendo.
- **A chave do servidor** fica em `remote-server.key` na pasta de dados
  (0600). Trocá-la (apagar o arquivo e reiniciar) obriga a parear de novo
  todos os dispositivos, porque eles a fixaram.

## HTTPS, sempre

- O app só aceita endereço `https://`, no pareamento e em todas as sessões;
  `http://`, endereço com usuário e senha, ou com consulta, é recusado na tela.
- O certificado é conferido do jeito normal do sistema (cadeia, nome do
  servidor, validade). Não existe opção para ignorar erro de certificado.
- O app não segue redirecionamento nenhum: um `3xx` (para `http://` ou para
  outro lugar) encerra a tentativa.
- As rotas remotas só atendem o proxy: com `WHATSAPP_MCP_REMOTE_TRUSTED_PROXY`
  (o nome do contêiner do Traefik, um IP ou uma faixa), outro contêiner da
  mesma rede Docker é recusado, mesmo dizendo que veio por HTTPS. Do proxy,
  elas exigem que a conexão tenha chegado por HTTPS (`X-Forwarded-Proto` com
  `https`, ou `wss` num WebSocket) e recusam qualquer pedido com `Origin`, que
  só uma página web mandaria. O `install-docker.sh` preenche o proxy confiável.
- Entre o proxy e o contêiner, o tráfego anda na rede privada do Docker (ou no
  loopback do servidor) e não é publicado como porta HTTP; mesmo ali, ele já
  vai cifrado pelo Noise.

## O que isto garante, e o que não

| Pergunta | Resposta |
|---|---|
| Um proxy ou CDN que termina o HTTPS lê as conversas? | Não. Ele vê o tamanho e o ritmo das mensagens, não o conteúdo, e não consegue alterá-las sem que a sessão caia. |
| O servidor lê as conversas? | Sim: ele é uma das pontas. Ele roda o WhatsApp e precisa ler para servir o MCP. A criptografia de ponta a ponta é entre o app e o serviço, não contra o serviço. |
| Alguém que copie o código de pareamento consegue entrar? | Só se usar antes do app, em até 10 minutos, uma vez. Nesse caso o app recebe recusa, e o dispositivo do intruso aparece na lista para ser revogado. Gere o código só na hora de colar. |
| Uma chave copiada identifica o mesmo Mac? | Não exatamente. A sessão prova a posse da chave privada do dispositivo, que fica no Keychain deste Mac e nunca trafega; copiá-la exige comprometer o Mac ou o Keychain. Não há atestação de hardware (Secure Enclave): a chave X25519 do Noise não pode morar nele. |
| E se o banco do servidor vazar? | Só o banco: ele tem só chaves públicas dos dispositivos, e não dá para se passar por eles. A chave privada do servidor está em `remote-server.key` (0600). Quem copiar a pasta de dados inteira, com o banco e essa chave, pode se passar pelo servidor para os apps e, por uma propriedade conhecida do padrão IK, também pelos dispositivos para o servidor. Nesse caso: revogue todos os dispositivos, apague `remote-server.key`, reinicie e pareie de novo. |
| Um programa no próprio Mac usa o MCP? | Sim, como no modo local: o `127.0.0.1:47821` do app atende os programas deste computador, com as mesmas regras que valeriam no servidor. Se o servidor tem token (`whatsapp-mcp token`), os programas do Mac também precisam dele para o MCP e para as ações da API; só a janela do próprio app dispensa. |

## Limites

- Até 16 handshakes ao mesmo tempo, até 64 pedidos ao mesmo tempo numa
  sessão; uma sessão sem nenhuma mensagem por 90 segundos (o app manda um
  ping a cada 25) é encerrada, e o app derruba uma sessão que para de
  responder aos pings.
- O corpo de um pedido que o servidor não lê não trava os outros pedidos da
  sessão: ele é descartado, e só aquele pedido falha.
- O nome do dispositivo é cortado em 64 caracteres.

## Decisões e alternativas

- **Por que Noise e não só um token no cabeçalho.** Um token copiado vale
  em qualquer lugar e, atrás de um proxy que termina o TLS, o conteúdo fica
  legível no proxy. Com o Noise não há segredo de longa duração no fio, o
  dispositivo prova posse da chave em cada sessão, e o proxy não lê nada.
- **Por que não mTLS.** Não é exigido nesta versão. O certificado de cliente
  terminaria no proxy (o mesmo problema do conteúdo legível) e pediria
  configurar o Traefik para cada instalação. A prova de posse fica no Noise.
- **Por que `flynn/noise`.** É a implementação de referência do Noise em Go
  (v1.1.0), usada em produção pelo Nebula e pelo go-libp2p. A alternativa
  considerada foi HPKE (`crypto/hpke`, da biblioteca padrão) no formato do
  OHTTP (RFC 9458), que deixaria a proteção contra replay e a autenticação do
  cliente para compor à parte; o Noise traz as duas no próprio padrão.
- **Referências:** OWASP Cheat Sheet Series, *REST Security*, *Session
  Management* e *Transport Layer Security*; a especificação do Noise
  (revisão 34).

## Testes

`internal/remote/remote_test.go` e `internal/daemon/remote_test.go` cobrem:
pareamento e pedidos pela sessão; código de uso único, expirado e com segredo
errado; o identificador do código, sozinho, não o gasta; chave do servidor
que não bate com o código (nada é registrado); `http://`, redirecionamento e
certificado não confiável recusados; rotas sem HTTPS, com `Origin` ou fora do
proxy confiável recusadas; dispositivo revogado (a sessão aberta cai na hora)
e chave desconhecida recusados; escopo `tools` restrito ao MCP; troca de
chave (a antiga para de valer; uma troca perdida pelo servidor volta à chave
atual); mensagem adulterada, mensagem repetida e handshake repetido
recusados; rekey; um corpo que ninguém lê não trava a sessão; nome cortado;
o token do servidor e as checagens do painel valendo para os programas do
computador do app, e só a janela do app tratada como janela; e o painel do
servidor pelo túnel, com o aviso de modo remoto, que não aparece no acesso
local. A primeira versão passou por uma revisão de segurança independente,
cujos achados estão corrigidos nesta.
