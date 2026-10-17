# Active Memory - GoWatch Session

**Data**: 2026-10-16  
**Status**: P2.1 entregue - Arquivo de configuração YAML funcionando (Fase 2, etapa 1 de 3)

---

## O que foi entregue

### P2.1 - Arquivo de configuração YAML
- `internal/config/config.go` implementou `Config` (com `LogsConfig`, `HistoryConfig`, `FiltersConfig`, `AlertsConfig`, `AlertRuleOverride`, `TracingConfig`, `AWSConfig`), `Default()`, `Load(LoadOptions)`, `Validate()`, `AlertsConfig.Rules()`, `FiltersConfig.MinLogLevel()` e `DefaultYAML()`. É um pacote folha: importa só stdlib, `go.yaml.in/yaml/v3`, `internal/alert` e `pkg/metrics`.
- Precedência: defaults < `~/.gowatch/config.yaml` < `./gowatch.yaml` < variáveis de ambiente (só AWS). Com `--config <arquivo>` valem só os defaults e esse arquivo (que precisa existir). Cada arquivo é decodificado sobre o resultado da camada anterior: listas são substituídas, mapas mesclados por chave, e `alerts.rules` combina campo a campo.
- Decodificação estrita (`KnownFields(true)`): chave com erro de digitação gera erro com arquivo e linha. Erros de YAML, validação e falha ao abrir a porta do receptor OTLP interrompem a inicialização com mensagem no stderr, antes de a TUI abrir.
- Endurecimento de segurança: o `./gowatch.yaml` (diretório de trabalho, pode vir de um repositório clonado e não confiável) não pode definir `aws.endpoint_url` nem um `tracing.otlp_receiver.address` fora de `127.0.0.1`, `[::1]` ou `localhost`; `Load` falha com erro que cita arquivo e chave e manda mover para `~/.gowatch/config.yaml` ou usar `--config`. A checagem decodifica o arquivo numa `Config` nova antes de mesclar. Esses valores continuam permitidos em `~/.gowatch/config.yaml` e em `--config`.
- Também rejeitados: `threshold`/`hysteresis` negativos, NaN ou infinitos, um segundo documento YAML (`---`) no mesmo arquivo e o mesmo ID de regra escrito em duas grafias. No `DefaultYAML()` o `hysteresis` das regras vem comentado, para que editar só o `threshold` funcione.
- Seções `theme`, `layout` e `keybindings` já são aceitas como dados brutos, mas só serão interpretadas e documentadas nas etapas 2 e 3 (P2.2 e P2.3).
- `internal/docker/collector.go` ganhou `Options` e `Configure()` (tail de logs, limite de histórico, regras de alerta, AWS). Valores zero mantêm o padrão; `AlertRules` nil mantém as regras padrão e uma lista vazia não-nil desliga os alertas.
- `internal/aws/client.go` ganhou `Options` e `NewAWSClientManagerWithOptions()`. Variáveis `AWS_*` continuam tendo prioridade; no modo `auto` a config com `region` ou `endpoint_url` também habilita a integração.
- `internal/ui/dashboard.go` ganhou `ApplyInitialFilters()` (filtros iniciais do config). `Esc` continua limpando todos os filtros.
- `cmd/gowatch/main.go` agora tem as flags `--config` e `--print-default-config`, inicia o receptor OTLP quando habilitado (com `Stop` no encerramento) e usa `refresh_interval` no ticker.
- Sem arquivo de config o comportamento é idêntico ao anterior (2s, tail 50, 60 pontos de histórico, regras padrão, detecção AWS por env, receptor OTLP desligado), com testes cobrindo esses valores.

### P5.1 - Sistema de Alertas & Limites (Alerting Engine)
- `internal/alert/types.go` implementou os modelos de dados `Alert`, `AlertRule`, `Severity` e `AlertStatus`, além de regras padrão (`HIGH_CPU`, `HIGH_MEM`, `OOM_KILLED`, `CONTAINER_DOWN`).
- `internal/alert/engine.go` implementou `AlertEngine` com histerese (resolução apenas abaixo do limite inferior), contadores de amostras consecutivas (*hitCounters*), cálculo seguro de memória (trata `MemLimit == 0`), normalização de CPU por core e auto-resolução de alertas de containers efêmeros.
- `internal/alert/engine_test.go` adicionou cobertura de testes unitários testando disparos consecutivos, histerese, divisão por zero, containers efêmeros e concorrência (`go test -race`).
- `internal/docker/collector.go` integrou o `globalAlertEngine` ao ciclo de amostragem `WatchContainers`.
- `internal/ui/dashboard.go` adicionou resumo compacto de alertas na barra `System Resources`, atalho `w` / `W` para abrir/fechar o modal de Alertas (`alertsModal`) e atalho `c` / `C` para limpar alertas resolvidos.

### P4.0 (Anterior - AWS Integration)
- Módulos AWS CloudWatch, Lambda, XRay, CloudFormation e alternância na TUI (`a`/`A`).

---

## Validações executadas
- `make build` OK (binário compilado sem erros em `bin/gowatch`)
- `make test` OK (todos os pacotes passando, incluindo os novos `cmd/gowatch` e `internal/config`)
- `go vet ./...` e `fieldalignment ./...` sem achados; `make go-sec` mantém apenas o G115 antigo em `internal/docker/collector.go` (conversão `int64 -> uint64` de `MemTotal`), que já existia antes da etapa
- Smoke test manual: `--print-default-config`, chave com erro de digitação, valor inválido, regra desconhecida e porta OTLP ocupada terminam com código de saída diferente de zero e mensagem clara

---

## Estado atual
- P2.1 (arquivo de configuração) entregue; Fase 5.1 entregue anteriormente.
- Próximas entregas da Fase 2: etapa 2 (P2.2 - temas e layout, com a flag `--theme`) e etapa 3 (P2.3 - atalhos remapeáveis, presets `default`/`vim` e menu de ajuda). Elas se apoiam nas structs brutas `ThemeConfig`, `LayoutConfig` e `KeybindingsConfig` já presentes em `internal/config`.
- Depois: Canais externos de notificação (P5.2 - Webhooks, Slack, SNS) e Ações de Gerenciamento de Containers (Fase 7).

---

## Notas técnicas importantes
- A configuração é lida uma única vez na inicialização (sem hot reload). `docker.Configure()` deve ser chamado em `main` antes de o goroutine de coleta começar.
- Os IDs de regra no YAML são minúsculos (`high_cpu`), enquanto `alert.DefaultRules()` usa maiúsculos (`HIGH_CPU`); a comparação ignora maiúsculas e minúsculas.
- `make field-fix` reordena campos de structs e quebra literais posicionais; use campos nomeados, inclusive nas tabelas de teste.
- O atalho `w` alterna exibição do modal `alertsModal`.
- O atalho `c` limpa alertas com status `RESOLVED`.
- REGRA ESTREITA: É estritamente proibido citar co-participação de IA, ferramentas de IA ou referências a IA na construção de código, mensagens de commit, pull requests ou documentação.
- REGRA ESTREITA: NUNCA criar commits ou executar git push sem a ordem expressa e individual do usuário.

---

**Encerrado em**: 2026-10-16  
**Retomar por**: Etapa 2 da Fase 2 (P2.2 - temas e layout), depois a etapa 3 (P2.3 - atalhos e ajuda), ou as fases P5.2 / Fase 7 conforme priorizado pelo usuário.
