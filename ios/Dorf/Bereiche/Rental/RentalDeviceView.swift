import SwiftUI

/// One device: what it is, what it costs, when it is taken — and the way to
/// book it.
///
/// The whole page is built around one sentence: the platform decides. „Ist es
/// frei?" is a question asked over the wire, and the booking button only wakes
/// up once the answer says so.
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
    @State private var signingIn = false

    private var signedIn: Bool {
        if case .angemeldet = umgebung.anmeldung.sitzung { return true }
        return false
    }

    var body: some View {
        List {
            if let hint = model.trouble?.message, model.trouble?.needsFreshSignIn != true {
                Section {
                    RentalNotice(text: hint) { Task { await model.load() } }
                }
            }

            pictureSection
            factsSection
            occupiedSection
            bookingSection
        }
        .accessibilityIdentifier("rental-device")
    }

    // MARK: Picture and text

    @ViewBuilder private var pictureSection: some View {
        if let url = model.device.imageURL {
            Section {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .success(let picture):
                        picture.resizable().scaledToFit()
                    case .failure:
                        EmptyView()
                    default:
                        ProgressView().frame(maxWidth: .infinity)
                    }
                }
                .frame(maxWidth: .infinity)
                .listRowInsets(EdgeInsets())
                .accessibilityHidden(true)
            }
        }
    }

    private var factsSection: some View {
        Section {
            if !model.device.description.isEmpty {
                Text(model.device.description)
                    .font(.body)
                    .fixedSize(horizontal: false, vertical: true)
            } else if !model.device.summary.isEmpty {
                Text(model.device.summary)
                    .font(.body)
                    .fixedSize(horizontal: false, vertical: true)
            }

            if let price = model.device.priceText {
                RentalFact(symbol: "eurosign.circle", text: price)
            }
            if let deposit = model.device.depositText {
                RentalFact(symbol: "lock.circle", text: deposit)
            }
            if let owner = model.device.ownerName {
                RentalFact(symbol: "person", text: "Verliehen von \(owner)")
            }
            if let pickup = model.device.pickupHint {
                RentalFact(symbol: "shippingbox", text: pickup)
            }
        }
    }

    @ViewBuilder private var occupiedSection: some View {
        Section("Schon vergeben") {
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
                    Label(period.text, systemImage: "calendar")
                        .font(.subheadline)
                }
            }
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
                    selection: Binding(get: { model.from }, set: { model.setFrom($0) }),
                    displayedComponents: .date
                )
                .accessibilityIdentifier("rental-from")

                DatePicker(
                    "Bis",
                    selection: Binding(get: { model.to }, set: { model.setTo($0) }),
                    in: model.from...,
                    displayedComponents: .date
                )
                .accessibilityIdentifier("rental-to")

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
                    if let price = answer.priceText {
                        RentalFact(symbol: "eurosign.circle", text: price)
                    }
                }

                if !model.device.bookable {
                    Text(model.device.unavailableNote
                        ?? "Dieses Gerät ist gerade nicht buchbar.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }

                TextField("Notiz an den Besitzer (freiwillig)", text: $model.note, axis: .vertical)
                    .lineLimit(1 ... 3)
                    .accessibilityIdentifier("rental-note")

                bookingButton
            } header: {
                Text("Wann brauchst du es?")
            } footer: {
                Text("Ob der Zeitraum frei ist, sagt der Maschinchenring — "
                    + "gebucht wird erst, wenn er zugestimmt hat.")
            }
        }
    }

    @ViewBuilder private var bookingButton: some View {
        if !signedIn {
            // No door in front of the catalogue, but there is one here: a
            // booking belongs to a person.
            VStack(alignment: .leading, spacing: 8) {
                Text("Zum Buchen brauchst du deine Rössing-ID.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                Button("Mit Rössing-ID anmelden") { Task { await signIn() } }
                    .disabled(signingIn)
                    .accessibilityIdentifier("rental-sign-in")
            }
        } else if model.trouble?.needsFreshSignIn == true {
            // The device has been signed in since before the rental platform
            // was part of the token. Only a fresh sign-in fixes that — saying
            // so beats an empty list and a shrug.
            VStack(alignment: .leading, spacing: 8) {
                Label(RentalError.signInOutdated.message, systemImage: "person.badge.key")
                    .font(.subheadline)
                Button("Neu anmelden") { Task { await signIn() } }
                    .disabled(signingIn)
                    .accessibilityIdentifier("rental-refresh-sign-in")
            }
            .accessibilityIdentifier("rental-sign-in-outdated")
        } else {
            Button {
                Task { await model.book() }
            } label: {
                HStack {
                    if model.booking { ProgressView().controlSize(.small) }
                    Text(model.booking ? "Wird gebucht …" : "Verbindlich anfragen")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(!model.canBook)
            .accessibilityIdentifier("rental-book")
        }
    }

    private func signIn() async {
        signingIn = true
        defer { signingIn = false }
        // A full sign-in, not a refresh: Zitadel only puts the rental
        // platform into the audience when the authorization request asks for
        // it, and a refreshed token keeps the audiences it was issued with.
        if await umgebung.anmeldung.anmelden() == .erfolg {
            model.clearTrouble()
            await umgebung.ichLaden()
        }
    }
}

/// One line of facts — the same arrangement for price, deposit, owner and
/// pickup, so the block does not look like four different things.
struct RentalFact: View {
    let symbol: String
    let text: String

    var body: some View {
        Label {
            Text(text).font(.subheadline)
        } icon: {
            Image(systemName: symbol).foregroundStyle(.tint)
        }
    }
}
