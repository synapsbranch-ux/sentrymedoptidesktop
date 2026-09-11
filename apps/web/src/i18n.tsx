import * as React from "react";
import { api } from "./api";
import { completeTranslations } from "./locale-packs";
import { printerTranslationCatalog, printerTranslations } from "./printer-translations";

export const supportedLanguages = [
  { code: "en", label: "English", nativeLabel: "English" },
  { code: "fr", label: "French", nativeLabel: "Français" },
  { code: "ht", label: "Haitian Creole", nativeLabel: "Kreyòl ayisyen" },
  { code: "pt", label: "Portuguese", nativeLabel: "Português" },
  { code: "es", label: "Spanish", nativeLabel: "Español" },
  { code: "de", label: "German", nativeLabel: "Deutsch" },
  { code: "zh-CN", label: "Chinese", nativeLabel: "简体中文" },
  { code: "ru", label: "Russian", nativeLabel: "Русский" },
  { code: "ja", label: "Japanese", nativeLabel: "日本語" },
  { code: "ko", label: "Korean", nativeLabel: "한국어" },
  { code: "id", label: "Indonesian", nativeLabel: "Bahasa Indonesia" },
] as const;

export type LanguageCode = (typeof supportedLanguages)[number]["code"];

const languageCodes = new Set<string>(supportedLanguages.map(({ code }) => code));

export function isLanguageCode(value: unknown): value is LanguageCode {
  return typeof value === "string" && languageCodes.has(value);
}

type Messages = Record<string, string>;

// English text is the stable message key and fallback. This lets domain modules
// migrate incrementally without ever rendering a missing-key token to staff.
const starterTranslations: Record<Exclude<LanguageCode, "en">, Messages> = {
  fr: {
    "Optical Clinic Management System": "Système de gestion de clinique optique",
    "Clinic Management": "Gestion de clinique",
    "Welcome back": "Bon retour",
    "Sign in with your clinic account to continue.": "Connectez-vous avec votre compte de clinique pour continuer.",
    "Email or username": "E-mail ou nom d’utilisateur",
    Password: "Mot de passe",
    "Hide password": "Masquer le mot de passe",
    "Show password": "Afficher le mot de passe",
    "Signing in…": "Connexion…",
    "Sign in": "Se connecter",
    "Clinic server available": "Serveur de la clinique disponible",
    "Clinic server unavailable": "Serveur de la clinique indisponible",
    "Checking clinic server…": "Vérification du serveur…",
    "Local-first clinic access. Your operational data remains on the SentryMed server.": "Accès local à la clinique. Vos données opérationnelles restent sur le serveur SentryMed.",
    "Connecting to clinic server…": "Connexion au serveur de la clinique…",
    Overview: "Aperçu", Dashboard: "Tableau de bord", Patients: "Patients", Appointments: "Rendez-vous", Consultations: "Consultations",
    Clinical: "Clinique", Prescriptions: "Ordonnances", "Optical / Lab": "Optique / Laboratoire", Documents: "Documents",
    Operations: "Opérations", Inventory: "Inventaire", "Stock takes": "Inventaires physiques", Purchasing: "Achats", "Point of Sale": "Point de vente", Billing: "Facturation",
    Finance: "Finances", Insurance: "Assurance", Reports: "Rapports", System: "Système", "Audit log": "Journal d’audit",
    Today: "Aujourd’hui", Queue: "File", POS: "PDV", Live: "En direct", Reconnecting: "Reconnexion",
    "Search patients, invoices, orders…": "Rechercher patients, factures, commandes…", "Search the clinic…": "Rechercher dans la clinique…",
    "Clinic search": "Recherche clinique", "Type at least two characters.": "Saisissez au moins deux caractères.", "No matching records.": "Aucun résultat correspondant.",
    "Sign out": "Se déconnecter", "Open navigation": "Ouvrir la navigation", "Close navigation": "Fermer la navigation", "Hide sidebar": "Masquer la barre latérale", "Show sidebar": "Afficher la barre latérale",
    Administration: "Administration", "Clinic identity, staff, LAN access, permissions and verified backups.": "Identité de la clinique, personnel, accès réseau, permissions et sauvegardes vérifiées.",
    Clinic: "Clinique", Appearance: "Apparence", Themes: "Thèmes", "Public display": "Écran public", Users: "Utilisateurs", Network: "Réseau", "Mobile & network": "Mobile et réseau", Backups: "Sauvegardes", Language: "Langue", Preview: "Aperçu",
    "Language & region": "Langue et région", "Application language": "Langue de l’application", "Choose the language used by every desktop and mobile client connected to this clinic server.": "Choisissez la langue utilisée par tous les clients ordinateur et mobile connectés à ce serveur.",
    "Save language": "Enregistrer la langue", "Saving…": "Enregistrement…", "Language saved for all clinic clients": "Langue enregistrée pour tous les clients de la clinique",
    "Could not save language": "Impossible d’enregistrer la langue", "Changes are propagated live to connected clients.": "Les modifications sont transmises en direct aux clients connectés.",
    "Clinic information": "Informations de la clinique", "Doctor account": "Compte médecin", "Local defaults": "Paramètres locaux", "Clinic name": "Nom de la clinique",
    Phone: "Téléphone", Email: "E-mail", Address: "Adresse", "Doctor full name": "Nom complet du médecin", Username: "Nom d’utilisateur",
    "Base currency": "Devise de base", "Clinic timezone": "Fuseau horaire de la clinique", "Backup destination": "Destination des sauvegardes",
    "Default local backup folder": "Dossier local par défaut", Choose: "Choisir", Back: "Retour", Continue: "Continuer", "Finish setup": "Terminer la configuration", "Preparing clinic…": "Préparation de la clinique…",
    "Network permission": "Autorisation réseau", "When the operating system asks, allow SentryMed Opti on private networks so clinic phones and laptops can connect.": "Lorsque le système le demande, autorisez SentryMed Opti sur les réseaux privés afin que les téléphones et ordinateurs de la clinique puissent se connecter.",
    "This section could not connect to the clinic server.": "Cette section n’a pas pu se connecter au serveur de la clinique.", "On a phone or tablet, verify the clinic Wi-Fi and keep the SentryMed desktop server running.": "Sur un téléphone ou une tablette, vérifiez le Wi-Fi de la clinique et gardez le serveur SentryMed actif.", "Reconnect and try again": "Reconnecter et réessayer",
    "Patient flow · Live clinic display": "Parcours patient · Écran clinique en direct", "Now serving": "En cours de prise en charge", "Current progress": "Progression actuelle", "Assigned to": "Assigné à", "Clinic team": "Équipe clinique", "No patient is being served right now.": "Aucun patient n’est pris en charge actuellement.", "Waiting queue": "File d’attente", Arrived: "Arrivé", "Upcoming appointments": "Prochains rendez-vous", "No upcoming appointment to display.": "Aucun prochain rendez-vous à afficher.", "Try again": "Réessayer", "Enter fullscreen": "Passer en plein écran", "Connecting to the clinic display…": "Connexion à l’écran de la clinique…", "Checked in": "Enregistré", "Waiting for nurse": "En attente de l’infirmier·ère", "Pre-test": "Pré-test", "Waiting for doctor": "En attente du médecin", "In consultation": "En consultation", Checkout: "Encaissement",
    Doctor: "Médecin", Nurse: "Infirmier·ère", Scheduled: "Planifié", Confirmed: "Confirmé", "Checked In": "Enregistré", Waiting: "En attente", Completed: "Terminé", Cancelled: "Annulé", "No Show": "Absent",
    "Appointments & waiting room": "Rendez-vous et salle d’attente", "Lab orders": "Commandes laboratoire", "Waiting room": "Salle d’attente", "New patient": "Nouveau patient", "New appointment": "Nouveau rendez-vous", "Start consultation": "Démarrer une consultation", "New item": "Nouvel article", "New supplier": "Nouveau fournisseur", "New order": "Nouvelle commande", "New user": "Nouvel utilisateur", "Record expense": "Enregistrer une dépense", "Save settings": "Enregistrer les paramètres", "Save theme": "Enregistrer le thème", Save: "Enregistrer", Cancel: "Annuler", Create: "Créer", Edit: "Modifier", Open: "Ouvrir", Download: "Télécharger", Print: "Imprimer", Refresh: "Actualiser", Search: "Rechercher", Name: "Nom", Role: "Rôle", Status: "Statut", Actions: "Actions", Date: "Date", Type: "Type", Patient: "Patient", Amount: "Montant", Description: "Description", Category: "Catégorie", Supplier: "Fournisseur", Payment: "Paiement", Invoice: "Facture", Notes: "Notes",
  },
  ht: {
    "Optical Clinic Management System": "Sistèm jesyon klinik optik",
    "Clinic Management": "Jesyon klinik", "Welcome back": "Byenvini ankò", "Sign in with your clinic account to continue.": "Konekte ak kont klinik ou pou kontinye.",
    "Email or username": "Imèl oswa non itilizatè", Password: "Modpas", "Hide password": "Kache modpas", "Show password": "Montre modpas", "Signing in…": "Koneksyon…", "Sign in": "Konekte",
    "Clinic server available": "Sèvè klinik la disponib", "Clinic server unavailable": "Sèvè klinik la pa disponib", "Checking clinic server…": "N ap verifye sèvè a…", "Connecting to clinic server…": "Koneksyon ak sèvè klinik la…",
    Overview: "Apèsi", Dashboard: "Tablo kontwòl", Patients: "Pasyan", Appointments: "Randevou", Consultations: "Konsiltasyon", Clinical: "Klinik", Prescriptions: "Preskripsyon", "Optical / Lab": "Optik / Laboratwa", Documents: "Dokiman",
    Operations: "Operasyon", Inventory: "Envantè", "Stock takes": "Konte estòk", Purchasing: "Acha", "Point of Sale": "Pwen vant", Billing: "Faktirasyon", Finance: "Finans", Insurance: "Asirans", Reports: "Rapò", System: "Sistèm", "Audit log": "Jounal odit",
    Today: "Jodi a", Queue: "Fil datant", Live: "An dirèk", Reconnecting: "Rekoneksyon", "Search patients, invoices, orders…": "Chèche pasyan, fakti, kòmand…", "Search the clinic…": "Chèche nan klinik la…",
    "Sign out": "Dekonekte", "Language & region": "Lang ak rejyon", "Application language": "Lang aplikasyon an", Language: "Lang", "Save language": "Anrejistre lang lan", "Saving…": "Anrejistreman…", "Language saved for all clinic clients": "Lang lan anrejistre pou tout aparèy klinik la",
    "Choose the language used by every desktop and mobile client connected to this clinic server.": "Chwazi lang tout òdinatè ak telefòn ki konekte ak sèvè klinik sa a ap itilize.", "Changes are propagated live to connected clients.": "Chanjman yo voye an dirèk sou aparèy ki konekte yo.",
    Clinic: "Klinik", Appearance: "Aparans", Themes: "Tèm", "Public display": "Ekran piblik", Users: "Itilizatè", Network: "Rezo", "Mobile & network": "Mobil ak rezo", Backups: "Sovgad", Administration: "Administrasyon", Preview: "Apèsi",
    "Clinic information": "Enfòmasyon klinik la", "Doctor account": "Kont doktè", "Local defaults": "Paramèt lokal", "Clinic name": "Non klinik la", Phone: "Telefòn", Email: "Imèl", Address: "Adrès", "Doctor full name": "Non konplè doktè a", Username: "Non itilizatè", "Base currency": "Lajan prensipal", "Clinic timezone": "Fizo orè klinik la", "Backup destination": "Kote sovgad yo", Choose: "Chwazi", Back: "Retounen", Continue: "Kontinye", "Finish setup": "Fini konfigirasyon", "Preparing clinic…": "N ap prepare klinik la…",
    "This section could not connect to the clinic server.": "Seksyon sa a pa t ka konekte ak sèvè klinik la.", "Reconnect and try again": "Rekonekte epi eseye ankò", "Patient flow · Live clinic display": "Sikilasyon pasyan · Ekran klinik an dirèk", "Now serving": "N ap sèvi kounye a", "Current progress": "Pwogrè aktyèl", "Assigned to": "Asiyen bay", "Clinic team": "Ekip klinik", "No patient is being served right now.": "Pa gen pasyan y ap sèvi kounye a.", "Waiting queue": "Fil datant", Arrived: "Rive", "Upcoming appointments": "Pwochen randevou", "No upcoming appointment to display.": "Pa gen pwochen randevou pou afiche.", "Try again": "Eseye ankò", "Enter fullscreen": "Mete ekran plen", "Connecting to the clinic display…": "Koneksyon ak ekran klinik la…", "Checked in": "Anrejistre", "Waiting for nurse": "Ap tann enfimyè", "Pre-test": "Pre-tès", "Waiting for doctor": "Ap tann doktè", "In consultation": "Nan konsiltasyon", Checkout: "Kesye", Doctor: "Doktè", Nurse: "Enfimyè", Scheduled: "Planifye", Confirmed: "Konfime", Waiting: "Ap tann", Completed: "Fini", Cancelled: "Anile",
    "Appointments & waiting room": "Randevou ak sal datant", "Lab orders": "Kòmand laboratwa", "Waiting room": "Sal datant", "New patient": "Nouvo pasyan", "New appointment": "Nouvo randevou", "Start consultation": "Kòmanse konsiltasyon", "New item": "Nouvo atik", "New supplier": "Nouvo founisè", "New order": "Nouvo kòmand", "New user": "Nouvo itilizatè", "Record expense": "Anrejistre depans", "Save settings": "Anrejistre paramèt yo", "Save theme": "Anrejistre tèm nan", Save: "Anrejistre", Cancel: "Anile", Create: "Kreye", Edit: "Modifye", Open: "Louvri", Download: "Telechaje", Print: "Enprime", Refresh: "Rafrechi", Search: "Chèche", Name: "Non", Role: "Wòl", Status: "Estati", Actions: "Aksyon", Date: "Dat", Type: "Tip", Patient: "Pasyan", Amount: "Kantite", Description: "Deskripsyon", Category: "Kategori", Supplier: "Founisè", Payment: "Peman", Invoice: "Fakti", Notes: "Nòt",
  },
  pt: {
    "Clinic Management": "Gestão da clínica", "Welcome back": "Bem-vindo de volta", "Sign in": "Entrar", "Signing in…": "Entrando…", "Email or username": "E-mail ou usuário", Password: "Senha",
    Overview: "Visão geral", Dashboard: "Painel", Patients: "Pacientes", Appointments: "Consultas agendadas", Consultations: "Consultas", Clinical: "Clínica", Prescriptions: "Prescrições", Documents: "Documentos", Operations: "Operações", Inventory: "Estoque", Purchasing: "Compras", Billing: "Faturamento", Finance: "Finanças", Insurance: "Seguro", Reports: "Relatórios", System: "Sistema", Users: "Usuários", Network: "Rede", Backups: "Backups", Language: "Idioma", "Language & region": "Idioma e região", "Application language": "Idioma do aplicativo", "Save language": "Salvar idioma", "Saving…": "Salvando…", Today: "Hoje", Queue: "Fila", Live: "Ao vivo", Reconnecting: "Reconectando", Back: "Voltar", Continue: "Continuar", Choose: "Escolher", "Appointments & waiting room": "Agendamentos e sala de espera", "Lab orders": "Pedidos de laboratório", "New patient": "Novo paciente", "New appointment": "Novo agendamento", "Start consultation": "Iniciar consulta", "New item": "Novo item", "New supplier": "Novo fornecedor", "New order": "Novo pedido", Save: "Salvar", Cancel: "Cancelar", Create: "Criar", Edit: "Editar", Open: "Abrir", Download: "Baixar", Print: "Imprimir", Refresh: "Atualizar", Search: "Pesquisar", Name: "Nome", Role: "Função", Status: "Status", Actions: "Ações", Date: "Data", Type: "Tipo", Patient: "Paciente", Doctor: "Médico", Nurse: "Enfermeiro", Amount: "Valor", Description: "Descrição", Category: "Categoria", Supplier: "Fornecedor", Payment: "Pagamento", Invoice: "Fatura", Notes: "Notas",
  },
  es: {
    "Clinic Management": "Gestión clínica", "Welcome back": "Bienvenido de nuevo", "Sign in": "Iniciar sesión", "Signing in…": "Iniciando sesión…", "Email or username": "Correo o usuario", Password: "Contraseña",
    Overview: "Resumen", Dashboard: "Panel", Patients: "Pacientes", Appointments: "Citas", Consultations: "Consultas", Clinical: "Clínica", Prescriptions: "Recetas", Documents: "Documentos", Operations: "Operaciones", Inventory: "Inventario", Purchasing: "Compras", Billing: "Facturación", Finance: "Finanzas", Insurance: "Seguro", Reports: "Informes", System: "Sistema", Users: "Usuarios", Network: "Red", Backups: "Copias de seguridad", Language: "Idioma", "Language & region": "Idioma y región", "Application language": "Idioma de la aplicación", "Save language": "Guardar idioma", "Saving…": "Guardando…", Today: "Hoy", Queue: "Cola", Live: "En vivo", Reconnecting: "Reconectando", Back: "Atrás", Continue: "Continuar", Choose: "Elegir", "Appointments & waiting room": "Citas y sala de espera", "Lab orders": "Órdenes de laboratorio", "New patient": "Nuevo paciente", "New appointment": "Nueva cita", "Start consultation": "Iniciar consulta", "New item": "Nuevo artículo", "New supplier": "Nuevo proveedor", "New order": "Nuevo pedido", Save: "Guardar", Cancel: "Cancelar", Create: "Crear", Edit: "Editar", Open: "Abrir", Download: "Descargar", Print: "Imprimir", Refresh: "Actualizar", Search: "Buscar", Name: "Nombre", Role: "Rol", Status: "Estado", Actions: "Acciones", Date: "Fecha", Type: "Tipo", Patient: "Paciente", Doctor: "Médico", Nurse: "Enfermero", Amount: "Importe", Description: "Descripción", Category: "Categoría", Supplier: "Proveedor", Payment: "Pago", Invoice: "Factura", Notes: "Notas",
  },
  de: {
    "Clinic Management": "Klinikverwaltung", "Welcome back": "Willkommen zurück", "Sign in": "Anmelden", "Signing in…": "Anmeldung…", "Email or username": "E-Mail oder Benutzername", Password: "Passwort",
    Overview: "Übersicht", Dashboard: "Dashboard", Patients: "Patienten", Appointments: "Termine", Consultations: "Konsultationen", Clinical: "Klinik", Prescriptions: "Rezepte", Documents: "Dokumente", Operations: "Betrieb", Inventory: "Inventar", Purchasing: "Einkauf", Billing: "Abrechnung", Finance: "Finanzen", Insurance: "Versicherung", Reports: "Berichte", System: "System", Users: "Benutzer", Network: "Netzwerk", Backups: "Sicherungen", Language: "Sprache", "Language & region": "Sprache und Region", "Application language": "Anwendungssprache", "Save language": "Sprache speichern", "Saving…": "Speichern…", Today: "Heute", Queue: "Warteschlange", Live: "Live", Reconnecting: "Verbindung wird wiederhergestellt", Back: "Zurück", Continue: "Weiter", Choose: "Auswählen", "Appointments & waiting room": "Termine und Wartezimmer", "Lab orders": "Laboraufträge", "New patient": "Neuer Patient", "New appointment": "Neuer Termin", "Start consultation": "Konsultation starten", "New item": "Neuer Artikel", "New supplier": "Neuer Lieferant", "New order": "Neue Bestellung", Save: "Speichern", Cancel: "Abbrechen", Create: "Erstellen", Edit: "Bearbeiten", Open: "Öffnen", Download: "Herunterladen", Print: "Drucken", Refresh: "Aktualisieren", Search: "Suchen", Name: "Name", Role: "Rolle", Status: "Status", Actions: "Aktionen", Date: "Datum", Type: "Typ", Patient: "Patient", Doctor: "Arzt", Nurse: "Pflegekraft", Amount: "Betrag", Description: "Beschreibung", Category: "Kategorie", Supplier: "Lieferant", Payment: "Zahlung", Invoice: "Rechnung", Notes: "Notizen",
  },
  "zh-CN": {
    "Clinic Management": "诊所管理", "Welcome back": "欢迎回来", "Sign in": "登录", "Signing in…": "正在登录…", "Email or username": "电子邮件或用户名", Password: "密码",
    Overview: "概览", Dashboard: "仪表板", Patients: "患者", Appointments: "预约", Consultations: "诊疗", Clinical: "临床", Prescriptions: "处方", Documents: "文档", Operations: "运营", Inventory: "库存", Purchasing: "采购", Billing: "账单", Finance: "财务", Insurance: "保险", Reports: "报告", System: "系统", Users: "用户", Network: "网络", Backups: "备份", Language: "语言", "Language & region": "语言和地区", "Application language": "应用语言", "Save language": "保存语言", "Saving…": "正在保存…", Today: "今天", Queue: "队列", Live: "实时", Reconnecting: "正在重新连接", Back: "返回", Continue: "继续", Choose: "选择",
  },
  ru: {
    "Clinic Management": "Управление клиникой", "Welcome back": "С возвращением", "Sign in": "Войти", "Signing in…": "Вход…", "Email or username": "Эл. почта или имя пользователя", Password: "Пароль",
    Overview: "Обзор", Dashboard: "Панель", Patients: "Пациенты", Appointments: "Записи", Consultations: "Консультации", Clinical: "Клиника", Prescriptions: "Рецепты", Documents: "Документы", Operations: "Операции", Inventory: "Запасы", Purchasing: "Закупки", Billing: "Счета", Finance: "Финансы", Insurance: "Страхование", Reports: "Отчёты", System: "Система", Users: "Пользователи", Network: "Сеть", Backups: "Резервные копии", Language: "Язык", "Language & region": "Язык и регион", "Application language": "Язык приложения", "Save language": "Сохранить язык", "Saving…": "Сохранение…", Today: "Сегодня", Queue: "Очередь", Live: "Онлайн", Reconnecting: "Повторное подключение", Back: "Назад", Continue: "Продолжить", Choose: "Выбрать",
  },
  ja: {
    "Clinic Management": "クリニック管理", "Welcome back": "おかえりなさい", "Sign in": "ログイン", "Signing in…": "ログイン中…", "Email or username": "メールまたはユーザー名", Password: "パスワード",
    Overview: "概要", Dashboard: "ダッシュボード", Patients: "患者", Appointments: "予約", Consultations: "診察", Clinical: "臨床", Prescriptions: "処方箋", Documents: "文書", Operations: "業務", Inventory: "在庫", Purchasing: "仕入れ", Billing: "請求", Finance: "財務", Insurance: "保険", Reports: "レポート", System: "システム", Users: "ユーザー", Network: "ネットワーク", Backups: "バックアップ", Language: "言語", "Language & region": "言語と地域", "Application language": "アプリの言語", "Save language": "言語を保存", "Saving…": "保存中…", Today: "今日", Queue: "待ち行列", Live: "ライブ", Reconnecting: "再接続中", Back: "戻る", Continue: "続行", Choose: "選択",
  },
  ko: {
    "Clinic Management": "진료소 관리", "Welcome back": "다시 오신 것을 환영합니다", "Sign in": "로그인", "Signing in…": "로그인 중…", "Email or username": "이메일 또는 사용자 이름", Password: "비밀번호",
    Overview: "개요", Dashboard: "대시보드", Patients: "환자", Appointments: "예약", Consultations: "진료", Clinical: "임상", Prescriptions: "처방전", Documents: "문서", Operations: "운영", Inventory: "재고", Purchasing: "구매", Billing: "청구", Finance: "재무", Insurance: "보험", Reports: "보고서", System: "시스템", Users: "사용자", Network: "네트워크", Backups: "백업", Language: "언어", "Language & region": "언어 및 지역", "Application language": "앱 언어", "Save language": "언어 저장", "Saving…": "저장 중…", Today: "오늘", Queue: "대기열", Live: "실시간", Reconnecting: "재연결 중", Back: "뒤로", Continue: "계속", Choose: "선택",
  },
  id: {
    "Clinic Management": "Manajemen klinik", "Welcome back": "Selamat datang kembali", "Sign in": "Masuk", "Signing in…": "Sedang masuk…", "Email or username": "Email atau nama pengguna", Password: "Kata sandi",
    Overview: "Ringkasan", Dashboard: "Dasbor", Patients: "Pasien", Appointments: "Janji temu", Consultations: "Konsultasi", Clinical: "Klinis", Prescriptions: "Resep", Documents: "Dokumen", Operations: "Operasional", Inventory: "Inventaris", Purchasing: "Pembelian", Billing: "Penagihan", Finance: "Keuangan", Insurance: "Asuransi", Reports: "Laporan", System: "Sistem", Users: "Pengguna", Network: "Jaringan", Backups: "Cadangan", Language: "Bahasa", "Language & region": "Bahasa dan wilayah", "Application language": "Bahasa aplikasi", "Save language": "Simpan bahasa", "Saving…": "Menyimpan…", Today: "Hari ini", Queue: "Antrean", Live: "Langsung", Reconnecting: "Menghubungkan kembali", Back: "Kembali", Continue: "Lanjutkan", Choose: "Pilih",
  },
};

const translations: Record<Exclude<LanguageCode, "en">, Messages> = Object.fromEntries(
  Object.entries(completeTranslations).map(([language, messages]) => [
    language,
    { ...messages, ...starterTranslations[language as Exclude<LanguageCode, "en">], ...printerTranslations[language as Exclude<LanguageCode, "en">] },
  ]),
) as unknown as Record<Exclude<LanguageCode, "en">, Messages>;

export const translationCatalog = Object.freeze([...new Set([...Object.keys(completeTranslations.fr), ...printerTranslationCatalog])]);
const englishMessages = new Set(translationCatalog);
const originalText = new WeakMap<Text, string>();
const originalAttributes = new WeakMap<Element, Map<string, string>>();
const translatableAttributes = ["alt", "aria-label", "placeholder", "title"] as const;

export function translateMessage(language: LanguageCode, message: string) {
  return language === "en" ? message : translations[language][message] ?? message;
}

export function missingTranslations(language: Exclude<LanguageCode, "en">) {
  return translationCatalog.filter((message) => !translations[language][message]?.trim());
}

function preserveWhitespace(source: string, replacement: string) {
  const leading = source.match(/^\s*/)?.[0] ?? "";
  const trailing = source.match(/\s*$/)?.[0] ?? "";
  return `${leading}${replacement}${trailing}`;
}

function skipped(element: Element | null) {
  return !element || Boolean(element.closest("[data-i18n-skip],script,style,code,pre,[contenteditable='true']"));
}

/**
 * Localizes legacy JSX text while pages are progressively migrated to t().
 * The source English is retained in WeakMaps, so switching language repeatedly
 * never translates a translation or alters patient-entered form values.
 */
export function localizeDOM(root: ParentNode, _language: LanguageCode, t: (message: string) => string) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let current = walker.nextNode();
  while (current) {
    const node = current as Text;
    if (!skipped(node.parentElement)) {
      const visible = node.data.trim();
      let source = originalText.get(node);
      if (source && visible !== t(source) && englishMessages.has(visible)) {
        source = visible;
        originalText.set(node, source);
      } else if (!source && englishMessages.has(visible)) {
        source = visible;
        originalText.set(node, source);
      }
      if (source) {
        const next = preserveWhitespace(node.data, t(source));
        if (node.data !== next) node.data = next;
      }
    }
    current = walker.nextNode();
  }

  const elements = root instanceof Element ? [root, ...root.querySelectorAll("*")] : [...root.querySelectorAll("*")];
  for (const element of elements) {
    if (skipped(element)) continue;
    let originals = originalAttributes.get(element);
    for (const attribute of translatableAttributes) {
      const value = element.getAttribute(attribute)?.trim();
      if (!value) continue;
      let source = originals?.get(attribute);
      if (source && value !== t(source) && englishMessages.has(value)) {
        source = value;
        originals?.set(attribute, source);
      } else if (!source && englishMessages.has(value)) {
        source = value;
        originals ??= new Map<string, string>();
        originals.set(attribute, source);
        originalAttributes.set(element, originals);
      }
      if (source) {
        const next = t(source);
        if (element.getAttribute(attribute) !== next) element.setAttribute(attribute, next);
      }
    }
  }
}

interface I18nValue {
  language: LanguageCode;
  apply(language: LanguageCode): void;
  refresh(): Promise<void>;
  t(message: string): string;
}

const I18nContext = React.createContext<I18nValue | null>(null);

function cachedLanguage(): LanguageCode {
  const saved = window.localStorage.getItem("sentrymed.language");
  return isLanguageCode(saved) ? saved : "en";
}

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [language, setLanguage] = React.useState<LanguageCode>(cachedLanguage);
  const apply = React.useCallback((next: LanguageCode) => {
    setLanguage(next);
    window.localStorage.setItem("sentrymed.language", next);
    document.documentElement.lang = next;
  }, []);
  const refresh = React.useCallback(async () => {
    const response = await api.get<{ language: string }>("/public/localization");
    if (isLanguageCode(response.language)) apply(response.language);
  }, [apply]);
  React.useEffect(() => { document.documentElement.lang = language; void refresh().catch(() => undefined); }, [language, refresh]);
  const t = React.useCallback((message: string) => translateMessage(language, message), [language]);
  React.useLayoutEffect(() => {
    const root = document.body;
    const applyTranslations = (node: Node = root) => {
      if (node instanceof Text) localizeDOM(node.parentNode ?? root, language, t);
      else if (node instanceof Element || node instanceof DocumentFragment || node === root) localizeDOM(node as ParentNode, language, t);
    };
    applyTranslations();
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        if (mutation.type === "characterData") applyTranslations(mutation.target);
        else if (mutation.type === "attributes") applyTranslations(mutation.target);
        else for (const node of mutation.addedNodes) applyTranslations(node);
      }
    });
    observer.observe(root, { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: [...translatableAttributes] });
    return () => observer.disconnect();
  }, [language, t]);
  const value = React.useMemo(() => ({ language, apply, refresh, t }), [language, apply, refresh, t]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const value = React.useContext(I18nContext);
  if (!value) throw new Error("useI18n must be used inside I18nProvider");
  return value;
}

// Error boundaries must never throw while rendering their own fallback, so they
// translate through this variant, which degrades to the English message key.
export function useOptionalI18n(): Pick<I18nValue, "t"> {
  return React.useContext(I18nContext) ?? { t: (message: string) => message };
}

export function translateNode(node: React.ReactNode, t: (message: string) => string): React.ReactNode {
  if (typeof node === "string") return t(node);
  if (Array.isArray(node)) return node.map((child) => translateNode(child, t));
  return node;
}
