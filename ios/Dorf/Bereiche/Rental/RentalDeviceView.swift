import SwiftUI

/// One device: what it is, what it costs, when it is taken — and the way to
/// book it.
///
/// The whole page is built around one sentence: the platform decides. „Ist es
/// frei?" is a question asked over the wire, and the booking button only wakes
/// up once the answer says so, for exactly the days on screen.
struct RentalDeviceView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    let device: RentalDevice
    @State private var model: RentalDeviceModel?

    var body: some View {
        Group {
            if let model {
                RentalDeviceDetail(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle(device.name)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let existing = model ?? RentalDeviceModel(
                device: device, source: .from(umgebung.rental)
            )
            model = existing
            await existing.load()
        }
    }
}

struct RentalDeviceDetail: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @ObservedObject var model: RentalDeviceModel

    private var signedIn: Bool {
        if case .angemeldet = umgebung.anmeldung.sitzung { return true }
        return false
    }

    var body: some View {
        List {
            if let trouble = model.trouble, !trouble.wantsSignIn {
                Section {
                    RentalNotice(text: trouble.message) { Task { await model.load() } }
                }
            }

            pictureSection
            factsSection
            occupiedSection
            bookingSection
            linkSection
        }
        .accessibilityIdentifier("rental-device")
    }

    // MARK: Picture and text

    @ViewBuilder private var pictureSection: some View {
        if !model.device.images.isEmpty {
            Section {
                // More than one picture: a strip one can push sideways.
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(model.device.images, id: \.self) { url in
                            AsyncImage(url: url) { phase in
                                switch phase {
                                case .success(let picture):
                                    picture.resizable().scaledToFill()
                                case .failure:
                                    Color.accentColor.opacity(0.10)
                                default:
                                    ProgressView()
                                }
                            }
                            .frame(width: 260, height: 195)
                            .clipShape(RoundedRectangle(cornerRadius: 12))
                        }
                    }
                    .padding(.vertical, 4)
                }
                .listRowInsets(EdgeInsets(top: 0, leading: 16, bottom: 0, trailing: 0))
                .accessibilityHidden(true)
            }
        }
    }

    private var factsSection: some View {
        Section {
            if !model.device.blocks.isEmpty {
                RentalMarkdownText(blocks: model.device.blocks)
            }

            // Every tariff the platform has, one line each. Deliberately no
            // sum: which tariff applies to which duration is nowhere written
            // down, and an invented rule here would be exactly the split
            // between web and app we avoid.
            ForEach(model.device.tariffs, id: \.self) { tariff in
                RentalFact(symbol: "eurosign.circle", text: tariff)
            }
            if let deposit = model.device.depositText {
                RentalFact(symbol: "lock.circle", text: deposit)
            }
            if !model.device.tags.isEmpty {
                RentalFact(symbol: "tag", text: model.device.tags.joined(separator: ", "))
            }
        } footer: {
            if model.device.tariffs.count > 1 {
                Text("Welcher Tarif für deinen Zeitraum gilt, sagt dir der "
                    + "Maschinchenring — die App rechnet das nicht aus.")
            }
        }
    }

    @ViewBuilder private var occupiedSection: some View {
        Section {
            if model.loading && model.occupied.isEmpty {
                HStack(spacing: 10) {
                    ProgressView()
                    Text("Belegung wird geholt …").foregroundStyle(.secondary)
                }
            } else if model.occupied.isEmpty {
                Text("Für die nächsten Wochen ist nichts eingetragen.")
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("rental-nothing-occupied")
            } else {
                ForEach(model.occupied) { period in
                    HStack(alignment: .firstTextBaseline) {
                        Label(period.text, systemImage: "calendar")
                            .font(.subheadline)
                        Spacer()
                        Text(period.kind.label)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("rental-occupied-\(period.id)")
                }
            }
        } header: {
            Text("Schon vergeben")
        } footer: {
            Text("Angefragt, vergeben oder gesperrt — belegt ist belegt.")
        }
    }

    // MARK: Booking

    @ViewBuilder private var bookingSection: some View {
        if let booking = model.confirmed {
            Section("Geschafft") {
                RentalBookingRow(booking: booking)
                Text("Die Entscheidung trifft, wem das Gerät gehört. "
                    + "Unter \u{201E}Meine Buchungen\u{201C} siehst du, wie es weitergeht.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            .accessibilityIdentifier("rental-booked")
        } else {
            Section {
                DatePicker(
                    "Von",
                    selection: Binding(get: { model.firstDay }, set: { model.setFirstDay($0) }),
                    displayedComponents: .date
                )
                .accessibilityIdentifier("rental-first-day")

                DatePicker(
                    "Bis einschließlich",
                    selection: Binding(get: { model.lastDay }, set: { model.setLastDay($0) }),
                    in: model.firstDay...,
                    displayedComponents: .date
                )
                .accessibilityIdentifier("rental-last-day")

                Button {
                    Task { await model.check() }
                } label: {
                    HStack {
                        if model.checking { ProgressView().controlSize(.small) }
                        Text(model.checking ? "Wird geprüft …" : "Ist es frei?")
                    }
                }
                .disabled(model.checking)
                .accessibilityIdentifier("rental-check")

                if let answer = model.answer {
                    Label(answer.message, systemImage: answer.free
                        ? "checkmark.circle.fill" : "xmark.circle.fill")
                        .font(.subheadline)
                        .foregroundStyle(answer.free ? Color.green : Color.secondary)
                        .accessibilityIdentifier("rental-answer")
                }

                TextField("Notiz für den Verleiher (freiwillig)", text: $model.notes, axis: .vertical)
                    .lineLimit(1 ... 3)
                    .accessibilityIdentifier("rental-notes")

                bookingButton
            } header: {
                Text("Wann brauchst du es?")
            } footer: {
                Text("Der Tag der Rückgabe ist der Tag nach dem letzten Leihtag — "
                    + "dann kann schon jemand anders anfangen. Ob der Zeitraum frei "
                    + "ist, sagt der Maschinchenring; gebucht ist erst, wenn der "
                    + "Verleiher zugestimmt hat.")
            }
        }
    }

    @ViewBuilder private var bookingButton: some View {
        if !signedIn {
            // No door in front of the catalogue, but there is one here: a
            // booking belongs to a person.
            RentalSignInNotice(trouble: RentalTrouble(RentalError.signInRequired)) {
                model.clearTrouble()
            }
        } else if let trouble = model.trouble, trouble.wantsSignIn {
            RentalSignInNotice(trouble: trouble) {
                model.clearTrouble()
            }
        } else {
            if !model.missingProfileFields.isEmpty {
                // The platform is missing something before it takes a
                // booking — its list, and the way to fill it in.
                VStack(alignment: .leading, spacing: 8) {
                    Text("Dem Maschinchenring fehlt noch: "
                        + model.missingProfileFields.joined(separator: ", ") + ".")
                        .font(.footnote)
                    NavigationLink("Profil ausfüllen") {
                        RentalProfileView()
                    }
                    .accessibilityIdentifier("rental-complete-profile")
                }
                .accessibilityIdentifier("rental-profile-incomplete")
            }

            Button {
                Task { await model.book() }
            } label: {
                HStack {
                    if model.booking { ProgressView().controlSize(.small) }
                    Text(model.booking ? "Wird angefragt …" : "Verbindlich anfragen")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(!model.canBook)
            .accessibilityIdentifier("rental-book")
        }
    }

    @ViewBuilder private var linkSection: some View {
        if model.device.webURL != nil || model.device.productURL != nil {
            Section {
                if let web = model.device.webURL {
                    Link(destination: web) {
                        Label("Im Maschinchenring öffnen", systemImage: "safari")
                    }
                    .accessibilityIdentifier("rental-web-link")
                }
                if let product = model.device.productURL {
                    Link(destination: product) {
                        Label("Seite des Herstellers", systemImage: "link")
                    }
                    .accessibilityIdentifier("rental-product-link")
                }
            }
        }
    }
}
