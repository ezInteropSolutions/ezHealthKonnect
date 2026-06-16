<?xml version="1.0" encoding="UTF-8"?>
<!--
  cda.xsl — C-CDA / CDA R2 viewer stylesheet
  Transforms a ClinicalDocument (urn:hl7-org:v3) to a clean HTML medical-record view.
  Handles: patient header, document info, all structured body sections,
  and all CDA NarrativeBlock elements (table, list, paragraph, content, br, linkHtml, etc.)
-->
<xsl:stylesheet version="1.0"
    xmlns:xsl="http://www.w3.org/1999/XSL/Transform"
    xmlns:n1="urn:hl7-org:v3"
    xmlns:sdtc="urn:hl7-org:sdtc"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    exclude-result-prefixes="n1 sdtc xsi">

  <xsl:output method="html" encoding="UTF-8" indent="no" omit-xml-declaration="yes"/>

  <!-- ═══════════════════════════════════════════════════════════════════════
       HELPERS
  ═══════════════════════════════════════════════════════════════════════════ -->

  <!-- Format TS date value (e.g. 19650314 or 20060106094825) → "Mar 14, 1965" -->
  <xsl:template name="formatDate">
    <xsl:param name="ts"/>
    <xsl:variable name="y" select="substring($ts,1,4)"/>
    <xsl:variable name="m" select="substring($ts,5,2)"/>
    <xsl:variable name="d" select="substring($ts,7,2)"/>
    <xsl:if test="string-length($ts) >= 8">
      <xsl:variable name="mon">
        <xsl:choose>
          <xsl:when test="$m='01'">Jan</xsl:when>
          <xsl:when test="$m='02'">Feb</xsl:when>
          <xsl:when test="$m='03'">Mar</xsl:when>
          <xsl:when test="$m='04'">Apr</xsl:when>
          <xsl:when test="$m='05'">May</xsl:when>
          <xsl:when test="$m='06'">Jun</xsl:when>
          <xsl:when test="$m='07'">Jul</xsl:when>
          <xsl:when test="$m='08'">Aug</xsl:when>
          <xsl:when test="$m='09'">Sep</xsl:when>
          <xsl:when test="$m='10'">Oct</xsl:when>
          <xsl:when test="$m='11'">Nov</xsl:when>
          <xsl:when test="$m='12'">Dec</xsl:when>
          <xsl:otherwise><xsl:value-of select="$m"/></xsl:otherwise>
        </xsl:choose>
      </xsl:variable>
      <xsl:value-of select="concat($mon,' ',number($d),', ',$y)"/>
    </xsl:if>
  </xsl:template>

  <!-- Format a person name element -->
  <xsl:template name="personName">
    <xsl:param name="name"/>
    <xsl:if test="$name/n1:prefix">
      <xsl:value-of select="normalize-space($name/n1:prefix)"/>
      <xsl:text> </xsl:text>
    </xsl:if>
    <xsl:value-of select="normalize-space($name/n1:given[1])"/>
    <xsl:if test="$name/n1:given[2]">
      <xsl:text> </xsl:text>
      <xsl:value-of select="normalize-space($name/n1:given[2])"/>
    </xsl:if>
    <xsl:if test="$name/n1:family">
      <xsl:text> </xsl:text>
      <xsl:value-of select="normalize-space($name/n1:family)"/>
    </xsl:if>
    <xsl:if test="$name/n1:suffix">
      <xsl:text>, </xsl:text>
      <xsl:value-of select="normalize-space($name/n1:suffix)"/>
    </xsl:if>
  </xsl:template>

  <!-- Map gender code to label -->
  <xsl:template name="genderLabel">
    <xsl:param name="code"/>
    <xsl:choose>
      <xsl:when test="$code='M'">Male</xsl:when>
      <xsl:when test="$code='F'">Female</xsl:when>
      <xsl:when test="$code='UN'">Unknown</xsl:when>
      <xsl:otherwise><xsl:value-of select="$code"/></xsl:otherwise>
    </xsl:choose>
  </xsl:template>

  <!-- ═══════════════════════════════════════════════════════════════════════
       ROOT / HTML SHELL
  ═══════════════════════════════════════════════════════════════════════════ -->

  <xsl:template match="/">
    <html lang="en">
      <head>
        <meta charset="UTF-8"/>
        <meta name="viewport" content="width=device-width, initial-scale=1"/>
        <title>
          <xsl:choose>
            <xsl:when test="n1:ClinicalDocument/n1:title">
              <xsl:value-of select="n1:ClinicalDocument/n1:title"/>
            </xsl:when>
            <xsl:otherwise>Clinical Document</xsl:otherwise>
          </xsl:choose>
        </title>
        <style>
          *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

          body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif;
            font-size: 13px;
            line-height: 1.55;
            color: #1f2937;
            background: #f1f5f9;
            padding: 14px;
          }

          /* ── Patient Header ────────────────────────────────────── */
          .pt-header {
            background: linear-gradient(135deg, #1e3a8a 0%, #1e40af 100%);
            color: #fff;
            border-radius: 10px;
            padding: 16px 20px;
            margin-bottom: 14px;
            display: flex;
            flex-wrap: wrap;
            gap: 14px;
            align-items: flex-start;
            box-shadow: 0 2px 8px rgba(30,58,138,0.25);
          }
          .pt-main { flex: 1; min-width: 200px; }
          .pt-name {
            font-size: 19px;
            font-weight: 700;
            letter-spacing: -0.02em;
            margin-bottom: 6px;
          }
          .pt-chips {
            display: flex;
            flex-wrap: wrap;
            gap: 5px;
          }
          .pt-chip {
            background: rgba(255,255,255,0.18);
            border: 1px solid rgba(255,255,255,0.25);
            border-radius: 4px;
            padding: 2px 8px;
            font-size: 11.5px;
            white-space: nowrap;
          }
          .pt-doc-info {
            text-align: right;
            font-size: 11.5px;
            opacity: 0.85;
            min-width: 150px;
          }
          .pt-doc-info .doc-title {
            font-size: 13px;
            font-weight: 600;
            opacity: 1;
            margin-bottom: 3px;
          }
          .pt-doc-info div { margin-bottom: 1px; }

          /* ── Section Cards ─────────────────────────────────────── */
          .sec-card {
            background: #fff;
            border: 1px solid #e2e8f0;
            border-radius: 8px;
            margin-bottom: 10px;
            overflow: hidden;
            box-shadow: 0 1px 3px rgba(0,0,0,0.05);
          }
          .sec-hdr {
            display: flex;
            align-items: center;
            padding: 9px 14px;
            background: #f8fafc;
            border-bottom: 1px solid #e2e8f0;
            cursor: pointer;
            user-select: none;
            gap: 8px;
          }
          .sec-hdr:hover { background: #f1f5f9; }
          .sec-accent {
            width: 3px;
            min-height: 18px;
            border-radius: 2px;
            background: #f472b6;
            flex-shrink: 0;
            align-self: stretch;
          }
          .sec-title {
            font-size: 13px;
            font-weight: 600;
            color: #1e3a8a;
            flex: 1;
          }
          .sec-chevron {
            color: #94a3b8;
            font-size: 10px;
            flex-shrink: 0;
            transition: transform 0.15s;
          }
          .sec-chevron.collapsed { transform: rotate(-90deg); }
          .sec-body {
            padding: 12px 14px;
            overflow-x: auto;
          }
          .sec-body.hidden { display: none; }

          /* ── CDA Tables ────────────────────────────────────────── */
          .cda-tbl {
            width: 100%;
            border-collapse: collapse;
            font-size: 12.5px;
            margin: 4px 0;
          }
          .cda-tbl caption {
            font-size: 11.5px;
            color: #64748b;
            font-style: italic;
            text-align: left;
            padding: 2px 0 4px;
          }
          .cda-tbl thead th {
            background: #f1f5f9;
            color: #374151;
            font-weight: 600;
            padding: 6px 10px;
            text-align: left;
            border-bottom: 2px solid #e2e8f0;
            font-size: 11.5px;
            text-transform: uppercase;
            letter-spacing: 0.04em;
            white-space: nowrap;
          }
          .cda-tbl tfoot td, .cda-tbl tfoot th {
            background: #f8fafc;
            font-size: 11px;
            color: #64748b;
            padding: 4px 10px;
            border-top: 1px solid #e2e8f0;
          }
          .cda-tbl tbody td {
            padding: 6px 10px;
            border-bottom: 1px solid #f1f5f9;
            vertical-align: top;
            color: #374151;
          }
          .cda-tbl tbody tr:nth-child(even) td { background: #fafafa; }
          .cda-tbl tbody tr:last-child td { border-bottom: none; }
          .cda-tbl tbody tr:hover td { background: #eff6ff; }
          /* TD as header (no thead) */
          .cda-tbl th:not(thead th) {
            background: #f8fafc;
            font-weight: 600;
            padding: 6px 10px;
            color: #4b5563;
            border-bottom: 1px solid #e2e8f0;
          }

          /* ── Lists ─────────────────────────────────────────────── */
          .cda-ul { list-style: disc; margin: 5px 0 5px 20px; }
          .cda-ol { list-style: decimal; margin: 5px 0 5px 20px; }
          .cda-ul li, .cda-ol li { margin: 3px 0; color: #374151; }

          /* ── Paragraphs ────────────────────────────────────────── */
          .cda-p { margin: 5px 0; color: #374151; }

          /* ── Inline content styles ─────────────────────────────── */
          .sc-bold  { font-weight: 700; }
          .sc-ital  { font-style: italic; }
          .sc-uline { text-decoration: underline; }
          .sc-fixed { font-family: 'Courier New', monospace; font-size: 0.9em; }
          .sc-sub   { vertical-align: sub; font-size: 0.8em; }
          .sc-sup   { vertical-align: super; font-size: 0.8em; }

          /* ── Misc ──────────────────────────────────────────────── */
          a           { color: #2563eb; text-decoration: none; }
          a:hover     { text-decoration: underline; }
          .no-content { color: #94a3b8; font-style: italic; font-size: 12px; padding: 6px 0; }
          .cda-media  { color: #94a3b8; font-size: 11px; font-style: italic; }
          .cda-fn     { font-size: 10.5px; color: #6b7280; vertical-align: super; }
        </style>
        <script>
          function toggleSec(id) {
            var body    = document.getElementById('sb-' + id);
            var chevron = document.getElementById('ch-' + id);
            if (!body) return;
            var collapsed = body.classList.toggle('hidden');
            if (chevron) chevron.classList.toggle('collapsed', collapsed);
          }
        </script>
      </head>
      <body>
        <xsl:apply-templates select="n1:ClinicalDocument"/>
      </body>
    </html>
  </xsl:template>

  <!-- ═══════════════════════════════════════════════════════════════════════
       CLINICAL DOCUMENT
  ═══════════════════════════════════════════════════════════════════════════ -->

  <xsl:template match="n1:ClinicalDocument">
    <xsl:call-template name="patientHeader"/>
    <xsl:apply-templates select="n1:component/n1:structuredBody/n1:component/n1:section"/>
  </xsl:template>

  <!-- ═══════════════════════════════════════════════════════════════════════
       PATIENT HEADER
  ═══════════════════════════════════════════════════════════════════════════ -->

  <xsl:template name="patientHeader">
    <xsl:variable name="pr"  select="n1:recordTarget[1]/n1:patientRole"/>
    <xsl:variable name="pt"  select="$pr/n1:patient"/>
    <xsl:variable name="addr" select="$pr/n1:addr[1]"/>

    <div class="pt-header">
      <div class="pt-main">
        <!-- Patient name -->
        <div class="pt-name">
          <xsl:call-template name="personName">
            <xsl:with-param name="name" select="$pt/n1:name[1]"/>
          </xsl:call-template>
          <xsl:if test="not(normalize-space($pt/n1:name[1]/n1:family))">
            Patient
          </xsl:if>
        </div>

        <!-- Demographics chips -->
        <div class="pt-chips">
          <xsl:if test="$pt/n1:birthTime/@value">
            <span class="pt-chip">
              <xsl:text>DOB: </xsl:text>
              <xsl:call-template name="formatDate">
                <xsl:with-param name="ts" select="$pt/n1:birthTime/@value"/>
              </xsl:call-template>
            </span>
          </xsl:if>

          <xsl:if test="$pt/n1:administrativeGenderCode">
            <span class="pt-chip">
              <xsl:choose>
                <xsl:when test="$pt/n1:administrativeGenderCode/@displayName">
                  <xsl:value-of select="$pt/n1:administrativeGenderCode/@displayName"/>
                </xsl:when>
                <xsl:otherwise>
                  <xsl:call-template name="genderLabel">
                    <xsl:with-param name="code" select="$pt/n1:administrativeGenderCode/@code"/>
                  </xsl:call-template>
                </xsl:otherwise>
              </xsl:choose>
            </span>
          </xsl:if>

          <xsl:if test="$pt/n1:raceCode/@displayName">
            <span class="pt-chip"><xsl:value-of select="$pt/n1:raceCode/@displayName"/></span>
          </xsl:if>

          <xsl:if test="$pt/n1:ethnicGroupCode/@displayName">
            <span class="pt-chip"><xsl:value-of select="$pt/n1:ethnicGroupCode/@displayName"/></span>
          </xsl:if>

          <xsl:if test="$pt/n1:languageCommunication/n1:languageCode/@code">
            <span class="pt-chip">
              <xsl:text>Lang: </xsl:text>
              <xsl:value-of select="$pt/n1:languageCommunication[1]/n1:languageCode/@code"/>
            </span>
          </xsl:if>

          <!-- MRN / IDs from patientRole -->
          <xsl:for-each select="$pr/n1:id[@extension and string-length(@extension) &lt; 30]">
            <span class="pt-chip">
              <xsl:text>ID: </xsl:text><xsl:value-of select="@extension"/>
            </span>
          </xsl:for-each>

          <!-- Address -->
          <xsl:if test="$addr/n1:city">
            <span class="pt-chip">
              <xsl:if test="$addr/n1:streetAddressLine">
                <xsl:value-of select="normalize-space($addr/n1:streetAddressLine[1])"/>
                <xsl:text>, </xsl:text>
              </xsl:if>
              <xsl:value-of select="normalize-space($addr/n1:city)"/>
              <xsl:if test="$addr/n1:state">
                <xsl:text> </xsl:text><xsl:value-of select="$addr/n1:state"/>
              </xsl:if>
              <xsl:if test="$addr/n1:postalCode">
                <xsl:text> </xsl:text><xsl:value-of select="$addr/n1:postalCode"/>
              </xsl:if>
            </span>
          </xsl:if>

          <!-- Phone -->
          <xsl:if test="$pr/n1:telecom[@use='HP' or @use='WP' or not(@use)]">
            <xsl:variable name="tel" select="$pr/n1:telecom[1]/@value"/>
            <xsl:if test="$tel">
              <span class="pt-chip">
                <xsl:value-of select="$tel"/>
              </span>
            </xsl:if>
          </xsl:if>
        </div>
      </div>

      <!-- Document info (right column) -->
      <div class="pt-doc-info">
        <xsl:if test="n1:title">
          <div class="doc-title"><xsl:value-of select="normalize-space(n1:title)"/></div>
        </xsl:if>
        <xsl:if test="n1:effectiveTime/@value">
          <div>
            <xsl:call-template name="formatDate">
              <xsl:with-param name="ts" select="n1:effectiveTime/@value"/>
            </xsl:call-template>
          </div>
        </xsl:if>
        <xsl:if test="n1:author[1]/n1:assignedAuthor/n1:assignedPerson/n1:name">
          <div>
            <xsl:text>Author: </xsl:text>
            <xsl:call-template name="personName">
              <xsl:with-param name="name" select="n1:author[1]/n1:assignedAuthor/n1:assignedPerson/n1:name"/>
            </xsl:call-template>
          </div>
        </xsl:if>
        <xsl:if test="n1:author[1]/n1:assignedAuthor/n1:representedOrganization/n1:name">
          <div><xsl:value-of select="n1:author[1]/n1:assignedAuthor/n1:representedOrganization/n1:name"/></div>
        </xsl:if>
        <xsl:if test="n1:custodian/n1:assignedCustodian/n1:representedCustodianOrganization/n1:name">
          <div><xsl:value-of select="n1:custodian/n1:assignedCustodian/n1:representedCustodianOrganization/n1:name"/></div>
        </xsl:if>
        <xsl:if test="n1:legalAuthenticator/n1:assignedEntity/n1:assignedPerson/n1:name">
          <div>
            <xsl:text>Signed: </xsl:text>
            <xsl:call-template name="personName">
              <xsl:with-param name="name" select="n1:legalAuthenticator/n1:assignedEntity/n1:assignedPerson/n1:name"/>
            </xsl:call-template>
          </div>
        </xsl:if>
      </div>
    </div>
  </xsl:template>

  <!-- ═══════════════════════════════════════════════════════════════════════
       SECTIONS
  ═══════════════════════════════════════════════════════════════════════════ -->

  <xsl:template match="n1:section">
    <xsl:variable name="sid" select="generate-id(.)"/>

    <div class="sec-card">
      <div class="sec-hdr" onclick="toggleSec('{$sid}')">
        <div class="sec-accent"/>
        <span class="sec-title">
          <xsl:choose>
            <xsl:when test="n1:title"><xsl:value-of select="normalize-space(n1:title)"/></xsl:when>
            <xsl:when test="n1:code/@displayName"><xsl:value-of select="n1:code/@displayName"/></xsl:when>
            <xsl:otherwise>Section</xsl:otherwise>
          </xsl:choose>
        </span>
        <span class="sec-chevron" id="ch-{$sid}">&#9660;</span>
      </div>
      <div class="sec-body" id="sb-{$sid}">
        <xsl:choose>
          <xsl:when test="n1:text">
            <xsl:apply-templates select="n1:text"/>
          </xsl:when>
          <xsl:otherwise>
            <span class="no-content">No narrative content in this section.</span>
          </xsl:otherwise>
        </xsl:choose>
      </div>
    </div>

    <!-- Recurse into nested sections -->
    <xsl:apply-templates select="n1:component/n1:section"/>
  </xsl:template>

  <!-- ═══════════════════════════════════════════════════════════════════════
       NARRATIVE BLOCK — text container
  ═══════════════════════════════════════════════════════════════════════════ -->

  <xsl:template match="n1:text">
    <xsl:apply-templates/>
  </xsl:template>

  <!-- ── Paragraph ─────────────────────────────────────────────────────── -->

  <xsl:template match="n1:paragraph">
    <p class="cda-p"><xsl:apply-templates/></p>
  </xsl:template>

  <!-- ── Line break ────────────────────────────────────────────────────── -->

  <xsl:template match="n1:br">
    <br/>
  </xsl:template>

  <!-- ── Lists ─────────────────────────────────────────────────────────── -->

  <xsl:template match="n1:list">
    <xsl:choose>
      <xsl:when test="@listType='ordered'">
        <ol class="cda-ol">
          <xsl:apply-templates select="n1:caption|n1:item"/>
        </ol>
      </xsl:when>
      <xsl:otherwise>
        <ul class="cda-ul">
          <xsl:apply-templates select="n1:caption|n1:item"/>
        </ul>
      </xsl:otherwise>
    </xsl:choose>
  </xsl:template>

  <xsl:template match="n1:list/n1:caption">
    <li style="list-style:none;font-size:11px;color:#64748b;font-style:italic;margin-bottom:3px;">
      <xsl:apply-templates/>
    </li>
  </xsl:template>

  <xsl:template match="n1:item">
    <li><xsl:apply-templates/></li>
  </xsl:template>

  <!-- ── Table ─────────────────────────────────────────────────────────── -->

  <xsl:template match="n1:table">
    <div style="overflow-x:auto;margin:4px 0;">
      <table class="cda-tbl">
        <xsl:apply-templates/>
      </table>
    </div>
  </xsl:template>

  <xsl:template match="n1:caption">
    <caption><xsl:apply-templates/></caption>
  </xsl:template>

  <xsl:template match="n1:col">
    <col>
      <xsl:if test="@width"><xsl:attribute name="width"><xsl:value-of select="@width"/></xsl:attribute></xsl:if>
    </col>
  </xsl:template>

  <xsl:template match="n1:colgroup">
    <colgroup><xsl:apply-templates/></colgroup>
  </xsl:template>

  <xsl:template match="n1:thead">
    <thead><xsl:apply-templates/></thead>
  </xsl:template>

  <xsl:template match="n1:tbody">
    <tbody><xsl:apply-templates/></tbody>
  </xsl:template>

  <xsl:template match="n1:tfoot">
    <tfoot><xsl:apply-templates/></tfoot>
  </xsl:template>

  <xsl:template match="n1:tr">
    <tr><xsl:apply-templates/></tr>
  </xsl:template>

  <xsl:template match="n1:th">
    <th>
      <xsl:if test="@colspan"><xsl:attribute name="colspan"><xsl:value-of select="@colspan"/></xsl:attribute></xsl:if>
      <xsl:if test="@rowspan"><xsl:attribute name="rowspan"><xsl:value-of select="@rowspan"/></xsl:attribute></xsl:if>
      <xsl:if test="@align">
        <xsl:attribute name="style">text-align:<xsl:value-of select="@align"/>;</xsl:attribute>
      </xsl:if>
      <xsl:apply-templates/>
    </th>
  </xsl:template>

  <xsl:template match="n1:td">
    <td>
      <xsl:if test="@colspan"><xsl:attribute name="colspan"><xsl:value-of select="@colspan"/></xsl:attribute></xsl:if>
      <xsl:if test="@rowspan"><xsl:attribute name="rowspan"><xsl:value-of select="@rowspan"/></xsl:attribute></xsl:if>
      <xsl:if test="@align">
        <xsl:attribute name="style">text-align:<xsl:value-of select="@align"/>;</xsl:attribute>
      </xsl:if>
      <xsl:apply-templates/>
    </td>
  </xsl:template>

  <!-- ── Inline content with styleCode ─────────────────────────────────── -->

  <xsl:template match="n1:content">
    <xsl:variable name="sc" select="@styleCode"/>
    <xsl:choose>
      <xsl:when test="contains($sc,'Bold') and contains($sc,'Italics')">
        <strong><em><xsl:apply-templates/></em></strong>
      </xsl:when>
      <xsl:when test="contains($sc,'Bold')">
        <strong class="sc-bold"><xsl:apply-templates/></strong>
      </xsl:when>
      <xsl:when test="contains($sc,'Italics') or contains($sc,'Italic')">
        <em class="sc-ital"><xsl:apply-templates/></em>
      </xsl:when>
      <xsl:when test="contains($sc,'Underline')">
        <span class="sc-uline"><xsl:apply-templates/></span>
      </xsl:when>
      <xsl:when test="contains($sc,'xFixed') or contains($sc,'monospace')">
        <code class="sc-fixed"><xsl:apply-templates/></code>
      </xsl:when>
      <xsl:when test="contains($sc,'Subscript')">
        <sub class="sc-sub"><xsl:apply-templates/></sub>
      </xsl:when>
      <xsl:when test="contains($sc,'Superscript')">
        <sup class="sc-sup"><xsl:apply-templates/></sup>
      </xsl:when>
      <xsl:otherwise>
        <span><xsl:apply-templates/></span>
      </xsl:otherwise>
    </xsl:choose>
  </xsl:template>

  <!-- ── Links ──────────────────────────────────────────────────────────── -->

  <xsl:template match="n1:linkHtml">
    <xsl:choose>
      <xsl:when test="@href">
        <a href="{@href}" target="_blank" rel="noopener">
          <xsl:apply-templates/>
        </a>
      </xsl:when>
      <xsl:otherwise>
        <span><xsl:apply-templates/></span>
      </xsl:otherwise>
    </xsl:choose>
  </xsl:template>

  <!-- ── Footnotes ──────────────────────────────────────────────────────── -->

  <xsl:template match="n1:footnote">
    <span class="cda-fn"><xsl:apply-templates/></span>
  </xsl:template>

  <xsl:template match="n1:footnoteRef">
    <span class="cda-fn">[<xsl:value-of select="@IDREF"/>]</span>
  </xsl:template>

  <!-- ── Multimedia ─────────────────────────────────────────────────────── -->

  <xsl:template match="n1:renderMultiMedia">
    <span class="cda-media">[media: <xsl:value-of select="@referencedObject"/>]</span>
  </xsl:template>

  <!-- ── Sub-elements inside n1:text that aren't listed above ──────────── -->

  <xsl:template match="n1:sub">
    <sub><xsl:apply-templates/></sub>
  </xsl:template>

  <xsl:template match="n1:sup">
    <sup><xsl:apply-templates/></sup>
  </xsl:template>

  <!-- ── Pass through text nodes ────────────────────────────────────────── -->

  <xsl:template match="text()">
    <xsl:value-of select="."/>
  </xsl:template>

</xsl:stylesheet>
