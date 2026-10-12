package i18n

import (
	"fmt"
	"strings"
	"sync"
)

var (
	mu           sync.RWMutex
	translations = map[string]map[string]string{
		"en": {
			"app_title":                    "MEGA Account Manager & Keep-Alive",
			"app_subtitle":                 "Automated account generator via Mailgun webhooks and periodic keep-alive daemon",
			"nav_dashboard":                "Dashboard",
			"nav_accounts":                 "Accounts",
			"nav_webhooks":                 "Mailgun Webhook",
			"nav_logs":                     "Activity Logs",
			"nav_settings":                 "Settings",
			"stat_total_accounts":          "Total Accounts",
			"stat_active_accounts":         "Active Accounts",
			"stat_pending_verification":    "Pending Verification",
			"stat_next_keepalive":          "Next Keep-Alive",
			"badge_verified":               "Verified",
			"badge_pending":                "Pending",
			"badge_failed":                 "Failed",
			"badge_active":                 "Active",
			"btn_generate":                 "Generate Accounts",
			"btn_generating":               "Generating...",
			"btn_trigger_keepalive":        "Run Keep-Alive Now",
			"btn_export_csv":               "Export CSV",
			"btn_export_json":              "Export JSON",
			"btn_export_rclone":            "Export rclone.conf",
			"btn_export_worker":            "Export Cloudflare Worker",
			"btn_export_html":              "Export Standalone HTML",
			"btn_fix_failed":               "Fix Failed Accounts",
			"btn_migrate":                  "Migrate / Import",
			"btn_simulate_webhook":         "Simulate Mailgun Webhook",
			"btn_copy":                     "Copy",
			"btn_copied":                   "Copied!",
			"btn_refresh":                  "Refresh",
			"btn_recheck":                  "Recheck",
			"btn_sync_index":              "Sync File Index",
			"btn_syncing":                 "Syncing...",
			"btn_cancel":                   "Cancel",
			"btn_import_submit":            "Import & Merge",
			"lbl_accounts_to_generate":     "Accounts to Generate",
			"lbl_custom_password":          "Custom Password (optional)",
			"lbl_pattern_preview":          "Email Pattern Preview",
			"lbl_domain":                   "Mail Domain",
			"lbl_keepalive_interval":       "Keep-Alive Interval (Days)",
			"lbl_last_check":               "Last Login Check",
			"lbl_email":                    "Email Address",
			"lbl_mega_password":            "MEGA Password",
			"lbl_status":                   "Status",
			"lbl_created_at":               "Created At",
			"lbl_actions":                  "Actions",
			"lbl_search":                   "Search accounts by email or status...",
			"lbl_webhook_url":              "Your Mailgun Webhook URL",
			"lbl_webhook_hint":             "Configure Mailgun Routes or Inbound Webhooks to POST to this endpoint:",
			"lbl_webhook_body_placeholder": "Paste Mailgun webhook payload or simulated email body with MEGA confirmation link...",
			"lbl_no_accounts":              "No accounts registered yet. Use the generator above or post a Mailgun webhook.",
			"lbl_generator_progress":       "Generating account %d of %d (%s)...",
			"lbl_rclone_policy":            "Union Create Policy (e.g., lus = least used space)",
			"lbl_rclone_name":              "Union Remote Name",
			"lbl_migration_title":          "Migrate Accounts Between Servers",
			"lbl_migration_hint":           "Select an accounts.csv or accounts.json file from your other server, or paste CSV/JSON lines below:",
			"lbl_migration_placeholder":    "Paste CSV rows (Email, MEGA Password...) or JSON array here...",
			"msg_keepalive_started":        "Keep-alive check started for all accounts.",
			"msg_keepalive_completed":      "Keep-alive cycle finished. Checked %d accounts.",
			"msg_generating_started":       "Started generating %d account(s) using pattern '%s' and domain '%s'.",
			"msg_account_registered":       "Initiated registration for %s. Awaiting Mailgun webhook confirmation.",
			"msg_account_verified":         "Successfully verified account %s via Mailgun webhook!",
			"msg_account_failed":           "Registration/verification failed for %s: %s",
			"msg_login_success":            "Successfully logged into account %s.",
			"msg_login_failed":             "Failed to log into account %s: %s",
			"msg_webhook_received":         "Received Mailgun webhook for recipient %s.",
			"msg_webhook_link_found":       "Extracted MEGA verification URL: %s",
			"msg_webhook_link_not_found":   "No MEGA confirmation link found in webhook body.",
			"msg_webhook_unauthorized":     "Invalid Mailgun webhook signature.",
			"msg_csv_saved":                "Saved account to accounts.csv.",
			"msg_fixing_failed_started":    "Starting login check to recover failed accounts...",
			"msg_sync_started":             "File index synchronization started in background.",
			"msg_sync_completed":           "File index synchronized successfully.",
			"msg_account_fixed":            "Successfully recovered account %s! Logged in and verified.",
			"msg_account_still_failed":     "Account %s could not be verified: %s",
			"msg_migration_success":        "Migration complete: %d imported, %d skipped.",
			"err_invalid_count":            "Count must be between 1 and 50.",
			"err_missing_domain":           "EMAIL_DOMAIN is not configured.",
			"err_account_not_found":        "Account not found for email: %s",
			"err_generation_in_progress":   "Account generation is already in progress. Please wait.",
			"lang_en":                      "English",
			"lang_pt":                      "Português",
			"lang_es":                      "Español",
		},
		"pt": {
			"app_title":                    "Gerenciador de Contas MEGA & Keep-Alive",
			"app_subtitle":                 "Gerador automatizado de contas via webhooks Mailgun e daemon de renovação periódica",
			"nav_dashboard":                "Painel",
			"nav_accounts":                 "Contas",
			"nav_webhooks":                 "Webhook Mailgun",
			"nav_logs":                     "Registros de Atividade",
			"nav_settings":                 "Configurações",
			"stat_total_accounts":          "Total de Contas",
			"stat_active_accounts":         "Contas Ativas",
			"stat_pending_verification":    "Aguardando Verificação",
			"stat_next_keepalive":          "Próximo Keep-Alive",
			"badge_verified":               "Verificada",
			"badge_pending":                "Pendente",
			"badge_failed":                 "Falhou",
			"badge_active":                 "Ativa",
			"btn_generate":                 "Gerar Contas",
			"btn_generating":               "Gerando...",
			"btn_trigger_keepalive":        "Executar Keep-Alive Agora",
			"btn_export_csv":               "Exportar CSV",
			"btn_export_json":              "Exportar JSON",
			"btn_export_rclone":            "Exportar rclone.conf",
			"btn_fix_failed":               "Corrigir Contas com Falha",
			"btn_migrate":                  "Migrar / Importar",
			"btn_simulate_webhook":         "Simular Webhook Mailgun",
			"btn_copy":                     "Copiar",
			"btn_copied":                   "Copiado!",
			"btn_refresh":                  "Atualizar",
			"btn_recheck":                  "Verificar",
			"btn_sync_index":              "Sincronizar Índice de Arquivos",
			"btn_syncing":                 "Sincronizando...",
			"btn_cancel":                   "Cancelar",
			"btn_import_submit":            "Importar & Mesclar",
			"lbl_accounts_to_generate":     "Quantidade de Contas",
			"lbl_custom_password":          "Senha Personalizada (opcional)",
			"lbl_pattern_preview":          "Prévia do Padrão de E-mail",
			"lbl_domain":                   "Domínio de E-mail",
			"lbl_keepalive_interval":       "Intervalo de Keep-Alive (Dias)",
			"lbl_last_check":               "Última Verificação",
			"lbl_email":                    "Endereço de E-mail",
			"lbl_mega_password":            "Senha do MEGA",
			"lbl_status":                   "Status",
			"lbl_created_at":               "Criado Em",
			"lbl_actions":                  "Ações",
			"lbl_search":                   "Buscar contas por e-mail ou status...",
			"lbl_webhook_url":              "Sua URL de Webhook do Mailgun",
			"lbl_webhook_hint":             "Configure rotas ou webhooks no Mailgun para enviar POST a este endpoint:",
			"lbl_webhook_body_placeholder": "Cole o payload do Mailgun ou corpo simulado com o link de confirmação do MEGA...",
			"lbl_no_accounts":              "Nenhuma conta cadastrada ainda. Use o gerador acima ou envie um webhook do Mailgun.",
			"lbl_generator_progress":       "Gerando conta %d de %d (%s)...",
			"lbl_rclone_policy":            "Política de Criação do Union (ex.: lus = menor espaço usado)",
			"lbl_rclone_name":              "Nome do Remoto Union",
			"lbl_migration_title":          "Migrar Contas Entre Servidores",
			"lbl_migration_hint":           "Selecione um arquivo accounts.csv ou accounts.json do outro servidor, ou cole o conteúdo abaixo:",
			"lbl_migration_placeholder":    "Cole linhas de CSV (Email, MEGA Password...) ou array JSON aqui...",
			"msg_keepalive_started":        "Verificação de keep-alive iniciada para todas as contas.",
			"msg_keepalive_completed":      "Ciclo de keep-alive finalizado. %d contas verificadas.",
			"msg_generating_started":       "Geração de %d conta(s) iniciada usando o padrão '%s' e domínio '%s'.",
			"msg_account_registered":       "Registro iniciado para %s. Aguardando confirmação do webhook Mailgun.",
			"msg_account_verified":         "Conta %s verificada com sucesso via webhook do Mailgun!",
			"msg_account_failed":           "Falha no registro/verificação de %s: %s",
			"msg_login_success":            "Login bem-sucedido na conta %s.",
			"msg_login_failed":             "Falha ao realizar login na conta %s: %s",
			"msg_webhook_received":         "Webhook do Mailgun recebido para o destinatário %s.",
			"msg_webhook_link_found":       "Link de verificação do MEGA extraído: %s",
			"msg_webhook_link_not_found":   "Nenhum link de confirmação do MEGA encontrado no webhook.",
			"msg_webhook_unauthorized":     "Assinatura do webhook Mailgun inválida.",
			"msg_csv_saved":                "Conta salva no arquivo accounts.csv.",
			"msg_fixing_failed_started":    "Iniciando verificação de login para recuperar contas com falha...",
			"msg_sync_started":             "Sincronização do índice de arquivos iniciada em segundo plano.",
			"msg_sync_completed":           "Índice de arquivos sincronizado com sucesso.",
			"msg_account_fixed":            "Conta %s recuperada com sucesso! Login realizado e verificada.",
			"msg_account_still_failed":     "Conta %s não pôde ser verificada: %s",
			"msg_migration_success":        "Migração concluída: %d importadas, %d ignoradas.",
			"err_invalid_count":            "A quantidade deve ser entre 1 e 50.",
			"err_missing_domain":           "EMAIL_DOMAIN não está configurado.",
			"err_account_not_found":        "Conta não encontrada para o e-mail: %s",
			"err_generation_in_progress":   "A geração de contas já está em andamento. Aguarde.",
			"lang_en":                      "English",
			"lang_pt":                      "Português",
			"lang_es":                      "Español",
		},
		"es": {
			"app_title":                    "Gestor de Cuentas MEGA & Keep-Alive",
			"app_subtitle":                 "Generador automatizado de cuentas mediante webhooks de Mailgun y demonio periódico de actividad",
			"nav_dashboard":                "Panel",
			"nav_accounts":                 "Cuentas",
			"nav_webhooks":                 "Webhook Mailgun",
			"nav_logs":                     "Registro de Actividad",
			"nav_settings":                 "Configuración",
			"stat_total_accounts":          "Cuentas Totales",
			"stat_active_accounts":         "Cuentas Activas",
			"stat_pending_verification":    "Pendientes de Verificación",
			"stat_next_keepalive":          "Próximo Keep-Alive",
			"badge_verified":               "Verificada",
			"badge_pending":                "Pendiente",
			"badge_failed":                 "Fallida",
			"badge_active":                 "Activa",
			"btn_generate":                 "Generar Cuentas",
			"btn_generating":               "Generando...",
			"btn_trigger_keepalive":        "Ejecutar Keep-Alive Ahora",
			"btn_export_csv":               "Exportar CSV",
			"btn_export_json":              "Exportar JSON",
			"btn_export_rclone":            "Exportar rclone.conf",
			"btn_fix_failed":               "Reparar Cuentas Fallidas",
			"btn_migrate":                  "Migrar / Importar",
			"btn_simulate_webhook":         "Simular Webhook Mailgun",
			"btn_copy":                     "Copiar",
			"btn_copied":                   "¡Copiado!",
			"btn_refresh":                  "Actualizar",
			"btn_recheck":                  "Recomprobar",
			"btn_sync_index":              "Sincronizar Índice de Archivos",
			"btn_syncing":                 "Sincronizando...",
			"btn_cancel":                   "Cancelar",
			"btn_import_submit":            "Importar & Fusionar",
			"lbl_accounts_to_generate":     "Cantidad de Cuentas",
			"lbl_custom_password":          "Contraseña Personalizada (opcional)",
			"lbl_pattern_preview":          "Vista Previa del Patrón",
			"lbl_domain":                   "Dominio de Correo",
			"lbl_keepalive_interval":       "Intervalo Keep-Alive (Días)",
			"lbl_last_check":               "Última Comprobación",
			"lbl_email":                    "Dirección de Correo",
			"lbl_mega_password":            "Contraseña de MEGA",
			"lbl_status":                   "Estado",
			"lbl_created_at":               "Fecha de Creación",
			"lbl_actions":                  "Acciones",
			"lbl_search":                   "Buscar cuentas por correo o estado...",
			"lbl_webhook_url":              "Tu URL de Webhook de Mailgun",
			"lbl_webhook_hint":             "Configura las rutas de Mailgun para enviar POST a este endpoint:",
			"lbl_webhook_body_placeholder": "Pega la carga útil o cuerpo simulado con el enlace de confirmación de MEGA...",
			"lbl_no_accounts":              "No hay cuentas registradas. Utiliza el generador o envía un webhook de Mailgun.",
			"lbl_generator_progress":       "Generando cuenta %d de %d (%s)...",
			"lbl_rclone_policy":            "Política de Creación del Union (ej.: lus = menor espacio usado)",
			"lbl_rclone_name":              "Nombre del Remoto Union",
			"lbl_migration_title":          "Migrar Cuentas Entre Servidores",
			"lbl_migration_hint":           "Selecciona un archivo accounts.csv o accounts.json de tu otro servidor, o pega el contenido debajo:",
			"lbl_migration_placeholder":    "Pega filas CSV (Email, MEGA Password...) o array JSON aquí...",
			"msg_keepalive_started":        "Verificación de actividad iniciada para todas las cuentas.",
			"msg_keepalive_completed":      "Ciclo de actividad finalizado. %d cuentas comprobadas.",
			"msg_generating_started":       "Generación de %d cuenta(s) iniciada con patrón '%s' y dominio '%s'.",
			"msg_account_registered":       "Registro iniciado para %s. Esperando confirmación por Mailgun webhook.",
			"msg_account_verified":         "¡Cuenta %s verificada exitosamente vía webhook de Mailgun!",
			"msg_account_failed":           "Error en registro/verificación de %s: %s",
			"msg_login_success":            "Inicio de sesión exitoso en la cuenta %s.",
			"msg_login_failed":             "Error al iniciar sesión en %s: %s",
			"msg_webhook_received":         "Webhook de Mailgun recibido para el destinatário %s.",
			"msg_webhook_link_found":       "Enlace de verificación de MEGA extraído: %s",
			"msg_webhook_link_not_found":   "No se encontró ningún enlace de confirmación en el webhook.",
			"msg_webhook_unauthorized":     "Firma del webhook de Mailgun no válida.",
			"msg_csv_saved":                "Cuenta guardada en accounts.csv.",
			"msg_fixing_failed_started":    "Iniciando comprobación de inicio de sesión para recuperar cuentas fallidas...",
			"msg_sync_started":             "Sincronización del índice de archivos iniciada en segundo plano.",
			"msg_sync_completed":           "Índice de archivos sincronizado con éxito.",
			"msg_account_fixed":            "¡Cuenta %s recuperada con éxito! Sesión iniciada y verificada.",
			"msg_account_still_failed":     "La cuenta %s no se pudo verificar: %s",
			"msg_migration_success":        "Migración completa: %d importadas, %d omitidas.",
			"err_invalid_count":            "La cantidad debe estar entre 1 e 50.",
			"err_missing_domain":           "EMAIL_DOMAIN no está configurado.",
			"err_account_not_found":        "No se encontró la cuenta con correo: %s",
			"err_generation_in_progress":   "La generación de cuentas ya está en progreso. Por favor espera.",
			"lang_en":                      "English",
			"lang_pt":                      "Português",
			"lang_es":                      "Español",
		},
	}
)

func NormalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if strings.HasPrefix(lang, "pt") {
		return "pt"
	}
	if strings.HasPrefix(lang, "es") {
		return "es"
	}
	return "en"
}

func T(lang, key string, args ...interface{}) string {
	mu.RLock()
	defer mu.RUnlock()

	norm := NormalizeLang(lang)
	dict, exists := translations[norm]
	if !exists {
		dict = translations["en"]
	}

	val, found := dict[key]
	if !found {
		if fallbackDict, ok := translations["en"]; ok {
			val = fallbackDict[key]
		}
	}
	if val == "" {
		val = key
	}

	if len(args) > 0 {
		return fmt.Sprintf(val, args...)
	}
	return val
}

func GetDictionary(lang string) map[string]string {
	mu.RLock()
	defer mu.RUnlock()

	norm := NormalizeLang(lang)
	res := make(map[string]string)
	for k, v := range translations["en"] {
		res[k] = v
	}
	if dict, exists := translations[norm]; exists {
		for k, v := range dict {
			res[k] = v
		}
	}
	return res
}

func GetAllLanguages() []string {
	return []string{"en", "pt", "es"}
}
