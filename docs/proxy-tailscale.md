# Proxy e Tailscale

É possível escolher o proxy de saída do WhatsApp, tanto no app quanto
na versão de linha de comando. A configuração vale para o pareamento, o sync,
os envios e as operações de mídia feitas pelo wacli. Reinicie o app ou o
serviço depois de mudar a configuração.

O MCP continua local. A escolha de proxy é passada só aos processos do wacli;
ela não altera o proxy dos downloads do app nem o roteamento do computador.

## Escolher a saída

Defina `whatsapp_proxy` no `config.json` da pasta de dados, preservando os
outros campos existentes:

```json
{
  "whatsapp_proxy": "socks5h://100.101.102.103:1080"
}
```

Esse endereço é ilustrativo: substitua pelo host e pela porta de um **proxy**
HTTP ou SOCKS5 que esteja funcionando no seu nó Tailscale. Só ter um IP
Tailscale não transforma o nó em proxy nem em exit node.

As pastas de dados do app estão em [Como funciona](como-funciona.md#configuração).
Na linha de comando, o padrão é `~/.whatsapp-mcp/config.json`, ou a pasta
indicada por `WHATSAPP_MCP_DATA`.

Também é possível escolher o proxy ao iniciar pelo terminal:

```bash
WHATSAPP_MCP_PROXY='socks5h://100.101.102.103:1080' whatsapp-mcp serve
```

`WHATSAPP_MCP_PROXY` vence o arquivo, inclusive quando está definida como
uma string vazia. Ao instalar o serviço com essa variável definida, a escolha
é salva no `config.json` privado da pasta de dados, preservando os outros
campos. A URL não é gravada no arquivo do serviço. Um app iniciado pelo ícone
pode não herdar o ambiente do terminal: nesse caso, use o arquivo.

| Valor | Comportamento |
|---|---|
| Vazio ou ausente | Mantém as variáveis de proxy herdadas pelo wacli, como no upstream. |
| `direct` | Remove as variáveis de proxy dos processos do wacli. O roteamento do sistema ainda vale. |
| `http://host:porta` | Proxy HTTP com CONNECT para HTTPS e WebSocket. |
| `https://host:porta` | Conexão TLS com o proxy HTTP. |
| `socks5://host:porta` ou `socks5h://host:porta` | Proxy SOCKS5; o transporte Go resolve o destino no proxy. |

URLs com usuário e senha também são aceitas, por exemplo
`http://USUARIO:SENHA@proxy.example:8080`. Caracteres reservados nas
credenciais precisam ser codificados para URL. O `config.json` escrito pelo
app tem permissão de acesso apenas para o dono no Unix; ele não é um cofre.

Uma configuração explícita substitui `NO_PROXY` herdado para impedir que uma
regra antiga como `*` faça o WhatsApp ignorar o proxy. Loopback continua direto,
para o socket de delegação e o relay de webhooks locais. Se o proxy falhar,
a conexão falha; esta configuração não acrescenta fallback direto.

Com um proxy explícito, envios de texto desativam a prévia automática de
links. O wacli 0.20.0 busca essas páginas com um cliente que ignora proxies;
desativar a busca evita que o site do link veja o IP direto desta máquina.
O texto e o link continuam sendo enviados. Na versão de linha de comando,
use wacli 0.20.0 ou uma versão que aceite `send text --no-preview`.

## Usar um IP residencial

Há duas formas de obter a saída residencial:

1. Usar um proxy residencial HTTP/SOCKS5 e configurar sua URL.
2. Usar um computador na conexão residencial como nó de saída Tailscale, ou
   rodar um proxy nele acessível pela rede Tailscale.

```text
WhatsApp MCP → wacli → proxy no nó Tailscale → internet residencial → WhatsApp
```

O WhatsApp verá o IP público de saída desse proxy/conexão residencial,
e não o endereço privado Tailscale do nó. Isso escolhe o caminho de rede;
o cliente continua sendo um dispositivo vinculado pelo protocolo WhatsApp Web.

## Quando o nó é um exit node

Se o seu nó já é um exit node aprovado no Tailscale, selecione-o no cliente
Tailscale da máquina que roda o app. Com a saída selecionada, o roteamento do
sistema já leva o tráfego público por esse nó. `whatsapp_proxy: "direct"`
desativa um proxy herdado, mas **não** desativa esse roteamento.

Esse modo também afeta os outros programas da máquina. Para aplicar uma saída
só ao wacli, use um endpoint de proxy; esta configuração restringe
essa escolha aos processos do WhatsApp.

O Tailscale também oferece SOCKS5/HTTP em modo userspace, útil para um
container ou uma instância dedicada. Esse proxy serve como endpoint para
`WHATSAPP_MCP_PROXY`; a escolha de exit node pertence à instância Tailscale.

Referências oficiais: [exit nodes](https://tailscale.com/docs/features/exit-nodes),
[rede em userspace](https://tailscale.com/docs/concepts/userspace-networking) e
[transporte HTTP do Go](https://pkg.go.dev/net/http#Transport).

## Estado desta implementação

- Configuração por arquivo ou variável de ambiente; ainda não há seletor de
  proxy/exit node no painel nem alteração automática do Tailscale.
- O app não inicia um proxy no nó remoto. Esse endpoint precisa existir.
- HTTP CONNECT e SOCKS5/SOCKS5h com autenticação foram exercitados por testes
  locais de HTTPS em processos filhos, além da passagem da configuração ao
  pareamento, sync e comandos.
- Em 2026-10-07, a implementação foi iniciada com wacli 0.20.0 real em pastas isoladas:
  recebeu QR code tanto por proxy HTTP autenticado quanto por um exit node
  Tailscale real. A saída brasileira e o HTTPS dos hosts do WhatsApp foram
  verificados nos dois caminhos. Nenhuma conta foi vinculada: envio, recepção
  e transferência de mídia com uma conta real ainda não foram testados.
